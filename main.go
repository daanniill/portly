package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	log.Println("Start portly")

	//define arguments
	// 127.0.0.1 is standard ip, basically localhost
	localAddress := flag.String(
		"listen",                     // name
		"127.0.0.1:8080",             // default
		"local address to listen on", //desc
	)

	remoteAddress := flag.String(
		"target",
		"127.0.0.1:9001",
		"remote address to target",
	)

	flag.Parse()

	// cancel contex when Ctrl+c or SIGTERM is received
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", *localAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *localAddress, err)
	}

	log.Printf("Portly forwarding %s → %s", *localAddress, *remoteAddress)

	var connections sync.WaitGroup

	go func() {
		<-ctx.Done()

		log.Println("shutdown signal received")
		log.Println("stopping new connections")

		if err := listener.Close(); err != nil {
			log.Printf("failed to close listener: %v", err)
		}
	}()

	if err := runForwarder(listener, *remoteAddress, &connections); err != nil {
		log.Fatalf("forwarder stopped: %v", err)
	}

	log.Println("waiting for active connections to finish")

	connections.Wait()

	log.Println("all connections finished")
	log.Println("Portly stopped cleanly")
}

func runForwarder(listener net.Listener, remoteAddress string, connections *sync.WaitGroup) error {
	// Handler listening function
	// will accept traffic at the bound port and run a goroutine as a non-blocking action to handle forwarding the request to the remote location
	for { // we want to continuously listen for requests and not immediately end the function execution
		client, err := listener.Accept()
		if err != nil {
			// don't return graceful shutdowns as errors
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("failed to accept connection: %v", err)
		}

		connections.Add(1)
		// Handle the actual forwarding to the remote
		go func() {
			defer connections.Done()
			handlePortForward(client, remoteAddress)
		}()

	}
}

func handlePortForward(client net.Conn, remoteAddress string) {
	log.Printf("forwarding connection from client %s to target %s", client.RemoteAddr(), remoteAddress)

	defer client.Close()

	target, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)
	if err != nil {
		log.Printf(
			"failed to connect client %s to target %s: %v",
			client.RemoteAddr(),
			remoteAddress,
			err,
		)
		return
	}
	defer target.Close()

	log.Printf(
		"connection opened: %s → %s",
		client.RemoteAddr(),
		remoteAddress,
	)

	// Requests are copied from the client to the target,
	// and responses are copied from the target back to the client.

	// Each direction reports its own error so a failure on one
	// side can never be masked by a nil result from the other.
	errClientToTarget := make(chan error, 1)
	errTargetToClient := make(chan error, 1)

	// STATS
	sent := make(chan int, 1)
	recieved := make(chan int, 1)
	start := time.Now()

	// Client request traffic:
	// client -> target
	go func() {
		bytes, err := io.Copy(target, client)
		sent <- int(bytes)
		errClientToTarget <- err
	}()

	// Target response traffic:
	// target -> client
	go func() {
		bytes, err := io.Copy(client, target)
		recieved <- int(bytes)
		errTargetToClient <- err
	}()

	sentBytes := <-sent
	receivedBytes := <-recieved
	clientToTargetErr := <-errClientToTarget
	targetToClientErr := <-errTargetToClient
	duration := time.Since(start)

	if clientToTargetErr != nil {
		log.Printf(
			"client→target copy for %s ended with an error: %v",
			client.RemoteAddr(),
			clientToTargetErr,
		)
	}

	if targetToClientErr != nil {
		log.Printf(
			"target→client copy for %s ended with an error: %v",
			client.RemoteAddr(),
			targetToClientErr,
		)
	}

	// ------------- PRINTING STATS -------------
	log.Printf(
		"connection closed: %s → %s",
		client.RemoteAddr(),
		remoteAddress,
	)

	log.Printf("sent: %d", sentBytes)
	log.Printf("received: %d", receivedBytes)
	log.Printf("duration: %d ms", duration.Milliseconds())
}
