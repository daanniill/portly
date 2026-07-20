package main

import (
	"log"
	"net"
	"testing"
)

// startTestForwarder starts the forwarder on an automatically selected port.
//
// Using port 0 tells the operating system to choose an available port,
// which prevents test failures caused by ports already being occupied.
func startTestForwarder(t *testing.T, targetAddress string) string {
	t.Helper() // mark this function as a helper

	listener, err := net.Listen("tcp", "127.0.0.1:0")// define a listener on any available port 
	if err != nil {
		log.Fatalf("failed tocreate forwarder listener: %v", err)
	}
	
	errCh := make(chan error, 1)

	go func() {
		errCh <- runForwarder(listener, targetAddress)
	}()
	
	// free up resources after running tests
	t.Cleanup(func() {
		// check if listener closed succesfully
		if err := listener.Close(); err != nil {
			t.Errorf("failed to close listener: %v", err)
		}
	})

	return listener.Addr().String() // returns address of where listener was opened on
}