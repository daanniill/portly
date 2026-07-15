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

	// These calls will do a bidirectional read/write across the open connections to the sockets opened to ensure that data is copied from the local server to the remote host
	// (and any responses from the target server are then copied back to the target server for additional handling)

	// Ensure the client gets the response data from the target
	go func() {
		io.Copy(client, target)
	}()
	
	// Ensure the target gets the request data from the client
	go func() {
		io.Copy(target, client)
	}()
}