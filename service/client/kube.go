package client

import (
	"encoding/json"
	"github.com/Boostport/kubernetes-vault/common"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
)

const (
	RoleAnnotation                = "pod.boostport.com/vault-approle"
	InitContainerAnnotation       = "pod.boostport.com/vault-init-container"
	InitContainerStatusAnnotation = "pod.beta.kubernetes.io/init-container-statuses"
)

type Kube struct {
	client    *kubernetes.Clientset
	namespace string
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

func (c *Kube) GetPods() ([]Pod, error) {

	p := []Pod{}

	pods, err := c.client.Core().Pods(c.namespace).List(v1.ListOptions{})

	if err != nil {
		return p, errors.Wrap(err, "could not list pods")
	}

	for _, pod := range pods.Items {

		convertedPod, err := convertToPod(&pod)

		if err == nil {
			p = append(p, convertedPod)
		}
	}

	return p, nil
}

func (c *Kube) WatchForPods() (<-chan Pod, chan<- struct{}, error) {

	events := make(chan Pod, 1024)
	stop := make(chan struct{})

	watcher, err := c.client.Core().Pods(c.namespace).Watch(v1.ListOptions{
		Watch: true,
	})

	if err != nil {
		return events, stop, errors.Wrap(err, "could not create watcher")
	}

	go c.watch(watcher, events, stop)

	return events, stop, nil
}

func (c *Kube) watch(watcher watch.Interface, events chan<- Pod, stop <-chan struct{}) {

	for {
		select {

		case event := <-watcher.ResultChan():

			if event.Type == "ADDED" || event.Type == "MODIFIED" {

				if pod, ok := event.Object.(*v1.Pod); ok {
					convertedPod, err := convertToPod(pod)

					if err == nil {
						events <- convertedPod
					}
				}
			}

		case <-stop:
			watcher.Stop()
			return
		}
	}
}

func convertToPod(pod *v1.Pod) (Pod, error) {

	initContainerReady := false
	role, hasRole := pod.Annotations[RoleAnnotation]
	initContainerName, hasInitContainerName := pod.Annotations[InitContainerAnnotation]
	initStatus, hasInitStatus := pod.Annotations[InitContainerStatusAnnotation]

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
			Name: pod.Name,
			Role: role,
			Ip:   pod.Status.PodIP,
			Port: common.InitContainerPort,
		}, nil
	}

	return Pod{}, errors.Errorf("Pod (%s) is not ready yet", pod.Name)
}

func (c *Kube) Discover(service string) ([]string, error) {

	ips := []string{}

	endpoints, err := c.client.Core().Endpoints(c.namespace).Get(service)

	if err != nil {
		return ips, errors.Wrapf(err, "could not get endpoints for the service %s", service)
	}

	for _, subset := range endpoints.Subsets {

		for _, endpoint := range subset.Addresses {
			ips = append(ips, endpoint.IP)
		}

		for _, endpoint := range subset.NotReadyAddresses {
			ips = append(ips, endpoint.IP)
		}
	}

	return ips, nil
}

func NewKube(namespace string) (*Kube, error) {

	config, err := rest.InClusterConfig()

	if err != nil {
		return nil, errors.Wrap(err, "could not create kubernetes config")
	}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		return nil, errors.Wrap(err, "could not create kubernetes client")
	}

	return &Kube{
		client:    clientset,
		namespace: namespace,
	}, nil
}
