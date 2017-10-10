package client

import (
	"context"
	"regexp"
	"time"

	"github.com/Boostport/kubernetes-vault/common"
	"github.com/cenkalti/backoff"
	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	RoleAnnotation                = "pod.boostport.com/vault-approle"
	InitContainerAnnotation       = "pod.boostport.com/vault-init-container"
	InitContainerStatusAnnotation = "pod.beta.kubernetes.io/init-container-statuses"
)

type Kube struct {
	client              *k8s.Client
	watchNamespaceRegex *regexp.Regexp
	logger              *logrus.Logger
}

type Pod struct {
	Name string
	Role string
	Ip   string
	Port int
}

type InitContainerStatus struct {
	Name  string
	State map[string]interface{}
}

func (k *Kube) GetPods() ([]Pod, error) {

	p := []Pod{}

	ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)

	pods, err := k.client.CoreV1().ListPods(ctx, k8s.AllNamespaces)

	if err != nil {
		return p, errors.Wrap(err, "could not list pods")
	}

	for _, pod := range pods.Items {

		if !k.isInWatchedNamespace(*pod.Metadata.Namespace) {
			continue
		}

		convertedPod, err := convertToPod(pod)

		if err == nil {
			p = append(p, convertedPod)
		}
	}

	return p, nil
}

func (k *Kube) WatchForPods() (<-chan Pod, chan<- struct{}, error) {

	events := make(chan Pod, 1024)
	stop := make(chan struct{})

	go k.watch(events, stop)

	return events, stop, nil
}

func (k *Kube) watch(events chan<- Pod, stop <-chan struct{}) {

	// Last observed resource version.
	resourceVersion := ""

	// Channels used for each instance of the watch.
	errCh := make(chan error, 1)

	var (
		watcher *k8s.CoreV1PodWatcher
		err     error
	)

	for {
		opt := k8s.ResourceVersion(resourceVersion)

		operation := func() error {
			ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)

			watcher, err = k.client.CoreV1().WatchPods(ctx, k8s.AllNamespaces, opt)

			if err != nil {
				k.logger.Errorf("Error watching pods: %s", err)
			}

			return err
		}

		err := backoff.Retry(operation, backoff.NewExponentialBackOff())

		if err != nil {
			k.logger.Errorf("Failed to start watcher: %s", err)
			continue
		}

		for {
			go func() {
				event, pod, err := watcher.Next()

				if err != nil {
					errCh <- err
					return
				}

				resourceVersion = *pod.Metadata.ResourceVersion

				if *event.Type == k8s.EventAdded || *event.Type == k8s.EventModified {
					if k.isInWatchedNamespace(*pod.Metadata.Namespace) {
						convertedPod, err := convertToPod(pod)

						if err == nil {
							events <- convertedPod
						}
					}
				}
			}()

			select {
			case err := <-errCh:
				k.logger.Errorf("Error watching pods: %s", err)
				break
			case <-stop:
				watcher.Close()
				return
			}
		}
	}
}

func convertToPod(pod *v1.Pod) (Pod, error) {

	initContainerReady := false
	role, hasRole := pod.Metadata.Annotations[RoleAnnotation]
	initContainerName, hasInitContainerName := pod.Metadata.Annotations[InitContainerAnnotation]
	podStatus := pod.GetStatus()

	if podStatus != nil {

		for _, initContainerStatus := range podStatus.InitContainerStatuses {

			if initContainerStatus.Name != nil && *initContainerStatus.Name == initContainerName {

				if initContainerStatus.State != nil && initContainerStatus.State.Running != nil {
					initContainerReady = true
					break
				}
			}
		}
	}

	if hasRole && hasInitContainerName && podStatus != nil && initContainerReady {
		return Pod{
			Name: *pod.Metadata.Name,
			Role: role,
			Ip:   *pod.Status.PodIP,
			Port: common.InitContainerPort,
		}, nil
	}

	return Pod{}, errors.Errorf("Pod (%s) is not ready yet", *pod.Metadata.Name)
}

func (k *Kube) Discover(serviceNamespace, service string) ([]string, error) {

	ips := []string{}

	ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)

	endpoints, err := k.client.CoreV1().GetEndpoints(ctx, service, serviceNamespace)

	if err != nil {
		return ips, errors.Wrapf(err, "could not get endpoints for the service %s", service)
	}

	for _, subset := range endpoints.Subsets {

		for _, endpoint := range subset.Addresses {
			ips = append(ips, *endpoint.Ip)
		}

		for _, endpoint := range subset.NotReadyAddresses {
			ips = append(ips, *endpoint.Ip)
		}
	}

	kubeDiscoveredNodes.Set(float64(len(ips)))

	return ips, nil
}

func (k *Kube) isInWatchedNamespace(namespace string) bool {
	return k.watchNamespaceRegex.MatchString(namespace)
}

func NewKube(watchNamespace string, logger *logrus.Logger) (*Kube, error) {

	var (
		r   *regexp.Regexp
		err error
	)

	if string(watchNamespace[0]) == "~" {
		r, err = regexp.Compile("(?i)" + string(watchNamespace[1:]))

		if err != nil {
			return nil, errors.Wrap(err, "invalid regex for watching namespace")
		}
	} else {
		r, err = regexp.Compile("(?i)^" + regexp.QuoteMeta(watchNamespace) + "$")

		if err != nil {
			return nil, errors.Wrap(err, "invalid watch namespace")
		}
	}

	client, err := k8s.NewInClusterClient()

	if err != nil {
		return nil, errors.Wrap(err, "could not create kubernetes client")
	}

	return &Kube{
		client:              client,
		watchNamespaceRegex: r,
		logger:              logger,
	}, nil
}
