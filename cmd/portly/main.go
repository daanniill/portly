package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/daanniill/portly/internal/forwarder"
)


func main() {
	log.Println("Start portly")

	// ------- FLAGS -------
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
	flag.Parse()

	//  ------- GRACEFUL SHUTDOWN -------
	// cancel contex when Ctrl+c or SIGTERM is received
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ------- LISTENER -------
	listener, err := net.Listen("tcp", *localAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *localAddress, err)
	}

	log.Printf("Portly forwarding %s → %s", listener.Addr().String(), *remoteAddress)

	var connections sync.WaitGroup

	go closeListenerOnShutdown(ctx, listener)

	f := &forwarder.Forwarder{
		TargetAddress: *remoteAddress,
		IdleTimeout:   *idleTimeout,
	}

	if err := f.Run(listener, &connections); err != nil {
		log.Fatalf("forwarder stopped: %v", err)
	}

	log.Println("waiting for active connections to finish")

	// notifying channel that is used to show connections are closed
	done := make(chan struct{}) // struct takes zero bytes of memory

	go func() {
		connections.Wait()
		close(done) //close channel
	}()

	select {
	case <-done: // if channel closes this case will run
		log.Println("all connections finished")
	case <-time.After(10 * time.Second):
		log.Println("shutdown timeout exceeded, exiting with connections still active")
	}
	log.Println("Portly stopped cleanly")
}

func closeListenerOnShutdown(ctx context.Context, listener net.Listener) {
	<-ctx.Done()

		log.Println("shutdown signal received")
		log.Println("stopping new connections")

		if err := listener.Close(); err != nil {
			log.Printf("failed to close listener: %v", err)
		}
}