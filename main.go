package main

import (
	"fmt"
	"io"
	"net"
)

var (
	localAddress string = "127.0.0.1:8080" // 127.0.0.1 is standard ip, basically localhost
	remoteForwarder string = "127.0.0.1:9000"
)

func main() {
	fmt.Println("Start portly")

	// Handle panics raised from the server
	defer func() {
		if e := recover(); e != nil { // creates a variable e := recover() (returns value cause by panic), then checks if its not nil
			fmt.Printf(
				"[CRITICAL] encountered a critical error, recovering from panic, error trace: %v",
				e,
			)
		}
	}() // defines anonymous function

	listener, err := net.Listen("tcp", localAddress) 
	if err != nil {
		panic(err)
	} 

	defer listener.Close()

	// Handler listening function
	// will accept traffic at the bound port and run a goroutine as a non-blocking action to handle forwarding the request to the remote location
	for { // we want to continuously listen for requests and not immediately end the function execution
		client, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		// Handle the actual forwarding to the remote
		go handlePortForward(client)
	}
}

func handlePortForward(client net.Conn) {
	fmt.Printf("forwarding connection from client %s to target %s\n", localAddress, remoteForwarder)

	target, err := net.Dial("tcp", remoteForwarder)
	if err != nil {
		panic(err)
	}

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