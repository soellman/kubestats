package main

import (
	"fmt"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
)

func pollKubernetes() {
	debugf("polling kubernetes API")
	go pollRCs()
	go pollSVCs()
	go pollNodes()
}

type kubeFunc func(*unversioned.Client) error

// Downstream func can ignore API errors
//   because API return structs on error are initialized properly
//   and we log if there was an error in the returned error
func withKube(f kubeFunc) {
	client, err := unversioned.New(&unversioned.Config{
		Host:    kubeAddr,
		Version: "v1",
	})

	if err != nil {
		debugf("client not created: %v\n", err)
		return
	}

	err = f(client)
	if err != nil {
		debugf("kubeFunc returned error: %v\n", err)
		incr("kubestats.errors")
	}
}

// find all RCs and report replica count
func pollRCs() {
	debugf("polling RCs")
	withKube(func(client *unversioned.Client) error {
		rcs, err := client.ReplicationControllers(api.NamespaceAll).List(labels.Everything())
		for _, i := range rcs.Items {
			gauge(fmt.Sprintf("rc.%s.%s", i.ObjectMeta.Namespace, i.ObjectMeta.Name), i.Status.Replicas)
		}
		return err
	})
}

// report endpoint addr count for a service
func showSvc(svc api.Service) {
	withKube(func(client *unversioned.Client) error {
		e := 0
		eps, err := client.Endpoints(svc.ObjectMeta.Namespace).List(labels.Set(svc.Spec.Selector).AsSelector())
		for _, ep := range eps.Items {
			for _, ss := range ep.Subsets {
				e = e + len(ss.Addresses)
			}
		}
		gauge(fmt.Sprintf("svc.%s.%s", svc.ObjectMeta.Namespace, svc.ObjectMeta.Name), e)
		return err
	})
}

// find all services
func pollSVCs() {
	debugf("polling services")
	withKube(func(client *unversioned.Client) error {
		svcs, err := client.Services(api.NamespaceAll).List(labels.Everything())
		for _, i := range svcs.Items {
			go showSvc(i)
		}
		return err
	})
}

// find all nodes and record gauge of statuses
// save map of node names to ips
func pollNodes() {
	debugf("polling nodes")
	withKube(func(client *unversioned.Client) error {
		nodes, err := client.Nodes().List(labels.Everything(), fields.Everything())
		status := make(map[string]int)
		names := make(map[string]string)
		for _, i := range nodes.Items {
			names[i.Status.Addresses[0].Address] = i.ObjectMeta.Name
			for _, c := range i.Status.Conditions {
				if c.Type == "Ready" {
					key := "unknown"
					switch c.Status {
					case "True":
						key = "ready"
					case "False":
						key = "notready"
					}
					status[key]++
				}
			}
		}
		for st, num := range status {
			gauge(fmt.Sprintf("nodes.status.%s", st), num)
		}
		pollPods(names)
		return err
	})
}

// get all pods and count per node
func pollPods(names map[string]string) {
	// with ips to names, look for all pods and count by IP and report by name
	withKube(func(client *unversioned.Client) error {
		ppn := make(map[string]int)
		pods, err := client.Pods(api.NamespaceAll).List(labels.Everything(), fields.Everything())
		for _, i := range pods.Items {
			ppn[i.Status.HostIP]++
		}
		for ip, p := range ppn {
			gauge(fmt.Sprintf("nodes.pods.%s", names[ip]), p)
		}
		return err
	})
}
