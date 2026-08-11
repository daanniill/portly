package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/daanniill/portly/internal/config"
	"github.com/daanniill/portly/internal/forwarder"
)

func main() {
	log.Println("Start portly")

	// ------- FLAGS -------
	configPath := flag.String(
		"config",
		"portly.yaml",
		"path to the Portly config file",
	)

	/* Remove flags for one forwarder
	// 127.0.0.1 is standard ip, basically localhost
	localAddress := flag.String(
		"listen",                     // name
		"127.0.0.1:0",                // default, listen on any available port
		"local address to listen on", //desc
	)
	remoteAddress := flag.String(
		"target",
		"127.0.0.1:9001",
		"remote address to target",
	)
	idleTimeout := flag.Duration(
		"idle-timeout",
		5*time.Minute,
		"close a connection after this long with no traffic; 0 disables",
	)
	*/

	flag.Parse()

	rules, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	//  ------- GRACEFUL SHUTDOWN -------
	// cancel contex when Ctrl+c or SIGTERM is received
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("starting Portly with %d rule(s)", len(rules))

	forwarders := make([]*forwarder.Forwarder, 0, len(rules))

	for _, rule := range rules {
		forwarders = append(forwarders, forwarder.New(rule))
	}

	var forwarderProcesses sync.WaitGroup

	errs := make(chan error, len(forwarders))

	for _, curForwarder := range forwarders {
		forwarderProcesses.Add(1)

		go func(f *forwarder.Forwarder) {
			defer forwarderProcesses.Done()

			if err := f.Start(ctx); err != nil {
				errs <- err
				stop()
			}
		}(curForwarder)
	}

	go func() {
		forwarderProcesses.Wait()
		close(errs)
	}()

	<-ctx.Done()

	log.Println("shutdown requested; stopping listeners")

	forwarderProcesses.Wait()

	log.Println("listeners stopped; waiting for active connections")

	for _, curForwarder := range forwarders {
		curForwarder.Wait()
	}

	for err := range errs {
		log.Printf("forwarder error: %v", err)
	}

	log.Println("Portly stopped cleanly")
}
