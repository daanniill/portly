package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

type ForwarderConfig struct {
	targetAddress string
	idleTimeout   time.Duration
}

func NewForwarderConfig() ForwarderConfig {
	return ForwarderConfig{
		targetAddress: "http://127.0.0.1:9001",
		idleTimeout:   2 * time.Second,
	}
}

// startTestForwarder starts the forwarder on an automatically selected port.
//
// Using port 0 tells the operating system to choose an available port,
// which prevents test failures caused by ports already being occupied.
func startTestForwarder(t *testing.T, forwarderCfg ForwarderConfig) net.Listener {
	t.Helper() // mark this function as a helper

	listener, err := net.Listen("tcp", "127.0.0.1:0") // define a listener on any available port
	if err != nil {
		log.Fatalf("failed to create forwarder listener: %v", err)
	}

	errCh := make(chan error, 1)

	var connections sync.WaitGroup

	go func() {
		errCh <- runForwarder(listener, forwarderCfg.targetAddress, &connections, forwarderCfg.idleTimeout)
	}()

	// free up resources after running tests
	t.Cleanup(func() {
		// tests may have already closed the listener themselves (e.g. to
		// simulate a shutdown signal), so ignore an already-closed listener
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("failed to close listener: %v", err)
		}
		connections.Wait()
	})

	return listener // returns listener
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

	targetCfg := NewForwarderConfig()
	targetCfg.targetAddress = getTargetAddress(t, targetServer.URL)
	forwarderAddress := startTestForwarder(t, targetCfg).Addr().String()

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

func TestForwarderHandlesConcurrentClients(t *testing.T) {
	// create a target test server using httptest
	targetServer := httptest.NewServer(
		// defining a handler function for this test server to handle http request
		// w is used to construct the HTTP response sent back to the client.
		// r contains information about the incoming request.
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)

	defer targetServer.Close()

	targetCfg := NewForwarderConfig()
	targetCfg.targetAddress = getTargetAddress(t, targetServer.URL)
	forwarderAddress := startTestForwarder(t, targetCfg).Addr().String()

	const requestCount = 50

	// initialize a client to send http requests
	client := newTestHTTPClient()
	errCh := make(chan error, requestCount)

	var wg sync.WaitGroup

	for i := 1; i <= requestCount; i++ {

		wg.Add(1)

		go func(requestNumber int) {
			defer wg.Done()

			response, err := client.Get("http://" + forwarderAddress)
			if err != nil {
				errCh <- fmt.Errorf("request %d through forwarder failed: %v", requestNumber, err)
				return
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil {
				errCh <- fmt.Errorf("request %d failed to read forwarded response: %v", requestNumber, err)
				return
			}

			if response.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("request %d expected status %d, but got %d", requestNumber, http.StatusOK, response.StatusCode)
				return
			}

			expectedBody := "concurrent response"
			if string(body) != expectedBody {
				errCh <- fmt.Errorf("expected body %q, but got %q", expectedBody, string(body))
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestForwarderSurvivesUnavailableTarget(t *testing.T) {
	// occupy a port with a listener then immediatley close it
	listener, err := net.Listen("tcp", "127.0.0.1:0") // define a listener on any available port
	if err != nil {
		log.Fatalf("failed to create forwarder listener: %v", err)
	}

	unavailableTarget := listener.Addr().String()
	// close the listener freeing the port
	if err := listener.Close(); err != nil {
		t.Fatalf("failed to close temporary target listener: %v", err)
	}

	// connect to the unavailable address

	unavailableTargetCfg := NewForwarderConfig()
	unavailableTargetCfg.targetAddress = unavailableTarget
	forwarderAddress := startTestForwarder(t, unavailableTargetCfg).Addr().String()
	client := newTestHTTPClient()

	// Try twice. The requests should fail, but the forwarder listener
	// should remain alive and accept the second connection.
	for attempt := 1; attempt <= 2; attempt++ {
		response, err := client.Get("http://" + forwarderAddress)

		if err == nil {
			response.Body.Close()
			t.Fatalf("attempt %d unexpectedly succeeded with unavailable target", attempt)
		}
	}
}

func TestIdleTimeout(t *testing.T) {
	// create a target test server using httptest
	targetServer := httptest.NewServer(
		// defining a handler function for this test server to handle http request
		// w is used to construct the HTTP response sent back to the client.
		// r contains information about the incoming request.
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)

	defer targetServer.Close()

	targetCfg := NewForwarderConfig()
	targetCfg.targetAddress = getTargetAddress(t, targetServer.URL)
	forwarderAddress := startTestForwarder(t, targetCfg).Addr().String()

	target, err := net.Dial("tcp", forwarderAddress)
	if err != nil {
		t.Fatalf("failed to dial forwarder: %v", err)
	}
	defer target.Close()

	time.Sleep(3 * time.Second)

	target.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := target.Read(buf); err == nil {
		t.Fatalf("expected connection to be closed after idle timeout, but read succeeded")
	}
}

func TestForwarderDeadline(t *testing.T) {
	// create a target test server using httptest
	targetServer := httptest.NewServer(
		// defining a handler function for this test server to handle http request
		// w is used to construct the HTTP response sent back to the client.
		// r contains information about the incoming request.
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)

	defer targetServer.Close()

	targetCfg := NewForwarderConfig()
	targetCfg.targetAddress = getTargetAddress(t, targetServer.URL)
	targetCfg.idleTimeout = 200 * time.Millisecond
	forwarderAddress := startTestForwarder(t, targetCfg).Addr().String()

	target, err := net.Dial("tcp", forwarderAddress)
	if err != nil {
		t.Fatalf("failed to dial forwarder: %v", err)
	}
	defer target.Close()

	// Create a context that automatically cancels after 1 second
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	target.SetWriteDeadline(time.Now().Add(2 * time.Second))

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-ticker.C:
			buf := make([]byte, 1)
			if _, err := target.Write(buf); err != nil {
				t.Fatalf("expected connection to be open")
			}
		}
	}

	time.Sleep(500 * time.Millisecond) // let idleTimeout (200ms) actually elapse

	target.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := target.Read(buf); err == nil {
		t.Fatalf("expected connection to be closed")
	}
}

// TestForwarderGracefulShutdown checks that closing the listener (as happens
// on a shutdown signal) stops runForwarder cleanly while letting an
// already in-flight connection finish instead of killing it.
func TestForwarderGracefulShutdown(t *testing.T) {
	// signals that the target has received the request, proving the
	// connection has actually been accepted and forwarded end-to-end
	// (not just sitting in the OS accept backlog) before we close the
	// listener below
	requestReceived := make(chan struct{})
	// holds the target's response until the test says it's safe to send it,
	// so the connection is genuinely in flight when we simulate shutdown
	releaseResponse := make(chan struct{})

	// create a target test server using httptest
	targetServer := httptest.NewServer(
		// defining a handler function for this test server to handle http request
		// w is used to construct the HTTP response sent back to the client.
		// r contains information about the incoming request.
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(requestReceived)
			<-releaseResponse

			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)

	defer targetServer.Close()

	targetCfg := NewForwarderConfig()
	targetCfg.targetAddress = getTargetAddress(t, targetServer.URL)
	targetCfg.idleTimeout = 200 * time.Millisecond
	forwarder := startTestForwarder(t, targetCfg)
	forwarderAddress := forwarder.Addr().String()

	target, err := net.Dial("tcp", forwarderAddress)
	if err != nil {
		t.Fatalf("failed to dial forwarder: %v", err)
	}
	defer target.Close()

	// start a request but don't read the response yet, so the connection
	// is still in flight when we simulate the shutdown signal below
	if _, err := target.Write([]byte("GET / HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// wait until the target has actually seen the request, so we know the
	// connection was accepted and forwarded before we close the listener
	<-requestReceived

	// simulate a shutdown signal: stop accepting new connections
	if err := forwarder.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	// let the target finish responding now that shutdown has been triggered
	close(releaseResponse)

	// the already-accepted connection above should still be served in full
	resp, err := http.ReadResponse(bufio.NewReader(target), nil)
	if err != nil {
		t.Fatalf("in-flight request failed after shutdown: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if string(body) != "concurrent response" {
		t.Errorf("expected body %q, got %q", "concurrent response", body)
	}

	// but a brand new connection should now be rejected
	if conn, err := net.Dial("tcp", forwarderAddress); err == nil {
		conn.Close()
		t.Fatalf("expected new connections to be rejected after shutdown")
	}
}
