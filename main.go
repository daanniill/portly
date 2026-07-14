package main

import (
	"fmt"
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
	for {
		localConnection, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		// Handle the actual forwarding to the remote
		go handlePortForward(localConnection)
	}
}