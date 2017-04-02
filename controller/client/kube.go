package client

import (
	"encoding/json"
	"regexp"

	"context"
	"time"

	"github.com/Boostport/kubernetes-vault/common"
	"github.com/Sirupsen/logrus"
	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"
	"github.com/pkg/errors"
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

	ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)

	watcher, err := k.client.CoreV1().WatchPods(ctx, k8s.AllNamespaces)

	if err != nil {
		return events, stop, errors.Wrap(err, "could not create watcher")
	}

	go k.watch(watcher, events, stop)

	return events, stop, nil
}

func (k *Kube) watch(watcher *k8s.CoreV1PodWatcher, events chan<- Pod, stop <-chan struct{}) {

	for {
		select {

		default:
			if event, pod, err := watcher.Next(); err != nil {
				k.logger.Errorf("Error getting next watch: %s", err)
			} else {
				if *event.Type == k8s.EventAdded || *event.Type == k8s.EventModified {
					if k.isInWatchedNamespace(*pod.Metadata.Namespace) {
						convertedPod, err := convertToPod(pod)

						if err == nil {
							events <- convertedPod
						}
					}
				}
			}

		case <-stop:
			watcher.Close()
			return
		}
	}
}

func convertToPod(pod *v1.Pod) (Pod, error) {

	initContainerReady := false
	role, hasRole := pod.Metadata.Annotations[RoleAnnotation]
	initContainerName, hasInitContainerName := pod.Metadata.Annotations[InitContainerAnnotation]
	initStatus, hasInitStatus := pod.Metadata.Annotations[InitContainerStatusAnnotation]

	if initStatus != "" {
		var initContainers []InitContainerStatus

		err := json.Unmarshal([]byte(initStatus), &initContainers)

		if err != nil {
			return Pod{}, err
		}

		for _, initContainer := range initContainers {

			if initContainer.Name == initContainerName {

				if _, ok := initContainer.State["running"]; ok {
					initContainerReady = true
					break
				}
			}
		}
	}

	if hasRole && hasInitContainerName && hasInitStatus && initContainerReady {
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
