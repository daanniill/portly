package main

import (
	"testing"
)

// startTestForwarder starts the forwarder on an automatically selected port.
//
// Using port 0 tells the operating system to choose an available port,
// which prevents test failures caused by ports already being occupied.
func startTestForwardewr(t *testing.T, targetAddress string) string {
	t.Helper() // mark this function as a helper

	listener, err := net.Listen("tcp", "127.0.0.1:0" // define a listener on any available port 
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *localAddress, err)
	} 
}