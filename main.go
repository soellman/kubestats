package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	statsd "github.com/happypancake/fsd"
	"golang.org/x/net/context"
)

func scheduler(ctx context.Context) {
	pollKubernetes()
	ticker := time.NewTicker(interval)

	for {
		select {
		case <-ctx.Done():
			debugf("scheduler cancelled\n")
			ticker.Stop()
			return
		case <-ticker.C:
			debugf("scheduler ticker ticked\n")
			pollKubernetes()
		}
	}
}

func workers() context.CancelFunc {

	ctx, cancel := context.WithCancel(context.Background())
	go scheduler(ctx)

	return cancel
}

var (
	debug        bool
	statsdAddr   string
	statsdPrefix string
	kubeAddr     string
	interval     time.Duration
)

func parseFlags() {
	flag.BoolVar(&debug, "debug", false, "debug logging")
	flag.StringVar(&statsdAddr, "statsdAddr", "localhost:8125", "statsd address")
	flag.StringVar(&statsdPrefix, "statsdPrefix", "kubernetes", "statsd prefix")
	flag.StringVar(&kubeAddr, "kubeAddr", "http://localhost:8080", "kubernetes address")
	flag.DurationVar(&interval, "interval", 60*time.Second, "statsd write interval")
	flag.Parse()

	log.Printf("Polling kubernetes API every %v\n", interval)
	log.Printf("Using %s as kubernetes API\n", kubeAddr)
	log.Printf("Using %s as statsd prefix\n", statsdPrefix)
}

func debugf(format string, args ...interface{}) {
	if debug {
		log.Printf(format, args...)
	}
}

func gauge(key string, value int) {
	statsd.Gauge(key, float64(value))
}

func incr(key string) {
	statsd.Count(key, 1)
}

func main() {
	// Initialize everything
	parseFlags()
	statsd.InitWithPrefixAndStaticConfig(statsdPrefix, statsdAddr)

	// Start up our worker routines
	cancel := workers()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Printf("Stopping.\n")
		cancel()
		// Give the context time to cancel
		time.Sleep(100 * time.Millisecond)
	}
}
