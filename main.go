package main

import (
	"fmt"
	"io"
	"log"
	"net"
)

var (
	localAddress string = "127.0.0.1:8080" // 127.0.0.1 is standard ip, basically localhost
	remoteAddress string = "127.0.0.1:9000"
)

func main() {
	fmt.Println("Start portly")

	listener, err := net.Listen("tcp", localAddress) 
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", localAddress, err)
	} 

	defer listener.Close()

	log.Printf("Portly forwarding %s → %s", localAddress, remoteAddress)

	// Handler listening function
	// will accept traffic at the bound port and run a goroutine as a non-blocking action to handle forwarding the request to the remote location
	for { // we want to continuously listen for requests and not immediately end the function execution
		client, err := listener.Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}

		// Handle the actual forwarding to the remote
		go handlePortForward(client)
	}
}

func handlePortForward(client net.Conn) {
	fmt.Printf("forwarding connection from client %s to target %s\n", localAddress, remoteAddress)

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

	// These calls will do a bidirectional read/write across the open connections to the sockets opened to ensure that data is copied from the local server to the remote host
	// (and any responses from the target server are then copied back to the target server for additional handling)

	// Client request traffic:
	// client -> target
	go func() {
		_, err := io.Copy(client, target)
		done <- err
	}()
	
	// Target response traffic:
	// target -> client
	go func() {
		_, err := io.Copy(target, client)
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