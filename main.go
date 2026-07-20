package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
)

func main() {
	log.Println("Start portly")

	//define arguments
	// 127.0.0.1 is standard ip, basically localhost
	localAddress := flag.String(
		"listen", // name
		"127.0.0.1:8080", // default
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

func runForwarder(listener net.Listener, remoteAddress string) error{
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

	// Buffered so both goroutines can report completion,
	// even after this function begins returning.
	done := make(chan error, 2)

	// Requests are copied from the client to the target,
	// and responses are copied from the target back to the client.

	// Client request traffic:
	// client -> target
	go func() {
		_, err := io.Copy(target, client)
		done <- err
	}()
	
	// Target response traffic:
	// target -> client
	go func() {
		_, err := io.Copy(client, target)
		done <- err
	}()

	copyErr := <-done

	if copyErr != nil {
		log.Printf(
			"connection %s ended with an error: %v",
			client.RemoteAddr(),
			copyErr,
		)
	}

	log.Printf(
		"connection closed: %s → %s",
		client.RemoteAddr(),
		remoteAddress,
	)
}