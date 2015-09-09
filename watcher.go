package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/watch"
)

func kubeEventWatch() (watch.Interface, error) {
	client, err := unversioned.New(&unversioned.Config{
		Host:    kubeAddr,
		Version: "v1",
	})

	if err != nil {
		debugf("client not created: %v\n", err)
		return nil, err
	}

	w, err := client.Events(api.NamespaceAll).Watch(labels.Everything(), fields.Everything(), "")
	if err != nil {
		debugf("kubeEventWatch returned error: %v\n", err)
		incr("kubestats.errors")
	}

	return w, err
}

func acquireWatch(ctx context.Context, out chan watch.Interface) {
	// try immediately
	w, err := kubeEventWatch()
	if err == nil {
		out <- w
		return
	}

	// then retry every 2s
	for {
		debugf("watch aquisition retrying 2s\n")
		ticker := time.NewTicker(2 * time.Second)
		select {
		case <-ctx.Done():
			debugf("watch aquisition cancelled\n")
			ticker.Stop()
			return
		case <-ticker.C:
			w, err := kubeEventWatch()
			if err == nil {
				out <- w
				return
			}
		}
	}
}

func watcher(ctx context.Context) {
	debugf("starting watcher\n")

	var closed = watch.Event{}
	var w watch.Interface
	var wchan = make(chan watch.Interface)
	defer close(wchan)

	for {

		go acquireWatch(ctx, wchan)

		select {
		case w = <-wchan:
			debugf("watch acquired\n")
		case <-ctx.Done():
			debugf("watcher cancelled\n")
			return
		}

	loop:
		for {
			select {
			case e := <-w.ResultChan():
				if e == closed {
					debugf("watcher closed channel with event: %+v\n", e)
					break loop
				}
				handleWatchEvent(e)
			case <-ctx.Done():
				w.Stop()
				debugf("watcher cancelled\n")
				return
			}
		}
	}
}

func handleWatchEvent(we watch.Event) {
	e, ok := we.Object.(*api.Event)
	if !ok {
		debugf("discarding unknown event\n")
		return
	}
	handleEvent(e)
}

func handleEvent(e *api.Event) {
	// don't do anything if e.LastTimestamp more than 5s before now
	// this ignores all events we get on first connect of a watch
	if e.LastTimestamp.UTC().Before(time.Now().UTC().Add(-5 * time.Second)) {
		debugf("ignoring old event: %s %q %s %s/%s\n", e.Source.Component, e.Reason, e.InvolvedObject.Kind, e.InvolvedObject.Namespace, e.InvolvedObject.Name)
		debugf("  from %v\n", e.LastTimestamp.UTC())
		debugf("  %s\n", strings.TrimSpace(e.Message))
		return
	}

	// Since people care most about failures, let's write it to the log
	fn := debugf
	if e.Reason == "failed" {
		fn = log.Printf
	} else {
		fn("Received event: %s %q %s %s/%s", e.Source.Component, e.Reason, e.InvolvedObject.Kind, e.InvolvedObject.Namespace, e.InvolvedObject.Name)
		if e.Count > 1 {
			fn(" (x%d)", e.Count)
		}
		fn("  %s\n", strings.TrimSpace(e.Message))
	}

	incr(fmt.Sprintf("event.%s.%s.%s", e.Source.Component, e.InvolvedObject.Kind, e.Reason))
}
