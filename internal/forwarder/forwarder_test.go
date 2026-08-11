package forwarder

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/daanniill/portly/internal/config"
)

// testForwarder wraps a Forwarder together with the machinery needed to
// start and stop it deterministically in tests.
type testForwarder struct {
	*Forwarder

	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

// newTestRule returns a RuntimeRule bound to a freshly allocated loopback
// port, so tests never collide over a fixed port number.
func newTestRule(t *testing.T, target string) config.RuntimeRule {
	t.Helper()

	return config.RuntimeRule{
		Name:        "test",
		Listen:      getFreeAddress(t),
		Target:      target,
		IdleTimeout: 2 * time.Second,
	}
}

// getFreeAddress asks the OS for an available loopback port and returns its
// address. The listener is closed immediately so a forwarder can bind to it.
func getFreeAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve a free port: %v", err)
	}

	address := listener.Addr().String()

	if err := listener.Close(); err != nil {
		t.Fatalf("failed to release reserved port: %v", err)
	}

	return address
}

// startTestForwarder starts a forwarder for rule in the background and
// registers cleanup to shut it down once the test finishes.
func startTestForwarder(t *testing.T, rule config.RuntimeRule) *testForwarder {
	t.Helper()

	f := New(rule)

	ctx, cancel := context.WithCancel(context.Background())

	tf := &testForwarder{
		Forwarder: f,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	go func() {
		tf.err = f.Start(ctx)
		close(tf.done)
	}()

	waitUntilListening(t, rule.Listen)

	t.Cleanup(func() {
		tf.stopAccepting(t)
		tf.Wait()
	})

	return tf
}

// stopAccepting cancels the forwarder's context and blocks until it has
// stopped accepting new connections. Safe to call more than once.
func (tf *testForwarder) stopAccepting(t *testing.T) {
	t.Helper()

	tf.cancel()
	<-tf.done

	if tf.err != nil {
		t.Errorf("forwarder %q exited with error: %v", tf.Name(), tf.err)
	}
}

// waitUntilListening blocks until address accepts TCP connections, so tests
// don't race the forwarder's goroutine that binds the listener.
func waitUntilListening(t *testing.T, address string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("forwarder did not start listening on %s in time", address)
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

// newTestHTTPClient creates a client to send requests
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
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("hello through forwarder")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)
	defer targetServer.Close()

	rule := newTestRule(t, getTargetAddress(t, targetServer.URL))
	startTestForwarder(t, rule)

	client := newTestHTTPClient()

	response, err := client.Get("http://" + rule.Listen)
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
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)
	defer targetServer.Close()

	rule := newTestRule(t, getTargetAddress(t, targetServer.URL))
	startTestForwarder(t, rule)

	const requestCount = 50

	client := newTestHTTPClient()
	errCh := make(chan error, requestCount)

	var wg sync.WaitGroup

	for i := 1; i <= requestCount; i++ {
		wg.Add(1)

		go func(requestNumber int) {
			defer wg.Done()

			response, err := client.Get("http://" + rule.Listen)
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
	// a reserved-then-released address that nothing is listening on
	unavailableTarget := getFreeAddress(t)

	rule := newTestRule(t, unavailableTarget)
	startTestForwarder(t, rule)

	client := newTestHTTPClient()

	// Try twice. The requests should fail, but the forwarder listener
	// should remain alive and accept the second connection.
	for attempt := 1; attempt <= 2; attempt++ {
		response, err := client.Get("http://" + rule.Listen)

		if err == nil {
			response.Body.Close()
			t.Fatalf("attempt %d unexpectedly succeeded with unavailable target", attempt)
		}
	}
}

func TestIdleTimeout(t *testing.T) {
	// create a target test server using httptest
	targetServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)
	defer targetServer.Close()

	rule := newTestRule(t, getTargetAddress(t, targetServer.URL))
	startTestForwarder(t, rule)

	target, err := net.Dial("tcp", rule.Listen)
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
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close") // close tcp connections when response finishes
			w.WriteHeader(http.StatusOK)          // send 200 for succesful connections

			if _, err := w.Write([]byte("concurrent response")); err != nil {
				t.Errorf("failed to write target response: %v", err)
			}
		}),
	)
	defer targetServer.Close()

	rule := newTestRule(t, getTargetAddress(t, targetServer.URL))
	rule.IdleTimeout = 200 * time.Millisecond
	startTestForwarder(t, rule)

	target, err := net.Dial("tcp", rule.Listen)
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

// TestForwarderGracefulShutdown checks that cancelling the forwarder's
// context (as happens on a shutdown signal) stops it from accepting new
// connections while letting an already in-flight connection finish instead
// of killing it.
func TestForwarderGracefulShutdown(t *testing.T) {
	// signals that the target has received the request, proving the
	// connection has actually been accepted and forwarded end-to-end
	// (not just sitting in the OS accept backlog) before we shut down below
	requestReceived := make(chan struct{})
	// holds the target's response until the test says it's safe to send it,
	// so the connection is genuinely in flight when we simulate shutdown
	releaseResponse := make(chan struct{})

	// create a target test server using httptest
	targetServer := httptest.NewServer(
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

	rule := newTestRule(t, getTargetAddress(t, targetServer.URL))
	rule.IdleTimeout = 200 * time.Millisecond
	tf := startTestForwarder(t, rule)

	target, err := net.Dial("tcp", rule.Listen)
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
	// connection was accepted and forwarded before we shut down
	<-requestReceived

	// simulate a shutdown signal: stop accepting new connections, and
	// block until the forwarder has actually done so
	tf.stopAccepting(t)

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
	if conn, err := net.Dial("tcp", rule.Listen); err == nil {
		conn.Close()
		t.Fatalf("expected new connections to be rejected after shutdown")
	}
}
