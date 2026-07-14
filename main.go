package main

import (
	"fmt"
	"io"
	"net"
)

var (
	localAddress string = "localhost:8081"
	remoteForwarder string = "https://example.com"
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
		localConnection, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		// Handle the actual forwarding to the remote
		go handlePortForward(localConnection)
	}
}

func handlePortForward(local net.Conn) {
	fmt.Printf("forwarding connection from local %s to remote %s\n", localAddress, remoteForwarder)

	remoteConnectionForwarded, err := net.Dial("tcp", remoteForwarder)
	if err != nil {
		panic(err)
	}

	// Ensure the local gets the response data from the remote
	go func() {
		io.Copy(local, remoteConnectionForwarded)
	}()


}