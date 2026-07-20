package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// startTestForwarder starts the forwarder on an automatically selected port.
//
// Using port 0 tells the operating system to choose an available port,
// which prevents test failures caused by ports already being occupied.
func startTestForwarder(t *testing.T, targetAddress string) string {
	t.Helper() // mark this function as a helper

	listener, err := net.Listen("tcp", "127.0.0.1:0") // define a listener on any available port
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

// creates a client to send requests
func newTestHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 3 * time.Second,
		// disable keep alives that reuse tcp connections to send requests
		// opens new tcp connection every time a client sends a request
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
}

func TestForwarderForwardsHTTPResponse(t *testing.T) {
	// create a target test server using httptest
	targetServer := httptest.NewServer(
		// defining a handler function for this test server to handle http request
		// w is used to construct the HTTP response sent back to the client.
		// r contains information about the incoming request.
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("hello through forwarder")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)

	defer targetServer.Close()

	targetAddress := getTargetAddress(t, targetServer.URL)
	forwarderAddress := startTestForwarder(t, targetAddress)

	// initialize a client to send http requests
	client := newTestHTTPClient()

	response, err := client.Get("http://" + forwarderAddress)
	if err != nil {
		t.Fatalf("request through forwarder failed: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("failed to read forwarded response: %v", err)
	}

	// ------------------- ACTUAL TESTS -------------------
	if response.StatusCode != http.StatusOK {
		t.Errorf(
			"expected status %d, but got %d",
			http.StatusOK,
			response.StatusCode,
		)
	}

	expectedBody := "hello through forwarder"

	if string(body) != expectedBody {
		t.Errorf(
			"expected body %q, but got %q",
			expectedBody,
			string(body),
		)
	}
}
