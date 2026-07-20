package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
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

	listener, err := net.Listen("tcp", *localAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *localAddress, err)
	}

	defer listener.Close()

	log.Printf("Portly forwarding %s → %s", *localAddress, *remoteAddress)

	if err := runForwarder(listener, *remoteAddress); err != nil {
		log.Fatalf("forwarder stopped: %v", err)
	}
}

func runForwarder(listener net.Listener, remoteAddress string) error {
	// Handler listening function
	// will accept traffic at the bound port and run a goroutine as a non-blocking action to handle forwarding the request to the remote location
	for { // we want to continuously listen for requests and not immediately end the function execution
		client, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %v", err)
		}

		// Handle the actual forwarding to the remote
		go handlePortForward(client, remoteAddress)
	}
}

func handlePortForward(client net.Conn, remoteAddress string) {
	log.Printf("forwarding connection from client %s to target %s", client.RemoteAddr(), remoteAddress)

	defer client.Close()

	target, err := net.Dial("tcp", remoteAddress)
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
