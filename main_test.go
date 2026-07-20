package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// getTargetAddress extracts "127.0.0.1:port" from an httptest URL.
func getTargetAddress(t *testing.T, serverURL string) string {
	t.Helper()

	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("failed to parse target URL: %v", err)
	}

	return parsedURL.Host
}

func TestForwarderForwardsHTTPResponse(t *testing.T) {
	targetServer := httptest.NewServer(
		// defining a handler function for this test server to handle http request
		// w is used to construct the HTTP response sent back to the client.
		// r contains information about the incoming request.
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK) // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)

	defer targetServer.Close()
}