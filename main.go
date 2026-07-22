package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type idleDeadline struct {
	timeout time.Duration
	client net.Conn
	target net.Conn
}

func (d *idleDeadline) refresh() error {
	if d.timeout <= 0{
		return nil
	}

	deadline := time.Now().Add(d.timeout)

	// returns err if there is an issue with setting deadline of client
	if err := d.client.SetDeadline(deadline); err != nil {
		return err
	}

	// returns err if there is an issue with setting deadline of target
	return d.target.SetDeadline(deadline)
}

// create a new struct that overwrites the read and write methods of net.Conn objects
// idleTimeoutConn becomes a wrapper around a real network connection that automatically refreshes the idle timeout whenever data is read or written.
type idleTimeoutConn struct {
	conn net.Conn
	deadline *idleDeadline
}

func (c *idleTimeoutConn) Read(buffer []byte) (int, error) {
	if err := c.deadline.refresh(); err != nil {
		return 0, err
	}

	return c.conn.Read(buffer)
}

func (c *idleTimeoutConn) Write(buffer []byte) (int, error) {
	if err := c.deadline.refresh(); err != nil {
		return 0, err
	}

	return c.conn.Write(buffer)
}

func main() {
	log.Println("Start portly")

	// ------- FLAGS ------- 
	// 127.0.0.1 is standard ip, basically localhost
	localAddress := flag.String(
		"listen",                     // name
		"127.0.0.1:0",                // default, listen on any available port
		"local address to listen on", //desc
	)
	remoteAddress := flag.String(
		"target",
		"127.0.0.1:9001",
		"remote address to target",
	)
	idleTimeout := flag.Duration(
		"idle-timeout",
		5*time.Minute,
		"close a connection after this long with no traffic; 0 disables",
	)
	flag.Parse()

	//  ------- GRACEFUL SHUTDOWN ------- 
	// cancel contex when Ctrl+c or SIGTERM is received
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ------- LISTENER ------- 
	listener, err := net.Listen("tcp", *localAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *localAddress, err)
	}

	log.Printf("Portly forwarding %s → %s", listener.Addr().String(), *remoteAddress)

	var connections sync.WaitGroup

	go func() {
		<-ctx.Done()

		log.Println("shutdown signal received")
		log.Println("stopping new connections")

		if err := listener.Close(); err != nil {
			log.Printf("failed to close listener: %v", err)
		}
	}()

	if err := runForwarder(listener, *remoteAddress, &connections, *idleTimeout); err != nil {
		log.Fatalf("forwarder stopped: %v", err)
	}

	log.Println("waiting for active connections to finish")

	connections.Wait()

	log.Println("all connections finished")
	log.Println("Portly stopped cleanly")
}

// -------------------- runs the forwarder  --------------------
func runForwarder(listener net.Listener, remoteAddress string, connections *sync.WaitGroup, idleTimeout time.Duration) error {
	// Handler listening function
	// will accept traffic at the bound port and run a goroutine as a non-blocking action to handle forwarding the request to the remote location
	for { // we want to continuously listen for requests and not immediately end the function execution
		client, err := listener.Accept()
		if err != nil {
			// don't return graceful shutdowns as errors
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("failed to accept connection: %v", err)
		}

		connections.Add(1)
		// Handle the actual forwarding to the remote
		go func() {
			defer connections.Done()
			handlePortForward(client, remoteAddress, idleTimeout)
		}()

	}
}

type copyResult struct {
	bytes int64
	err error
}

// -------------------- handles the port forwarding logic --------------------
func handlePortForward(client net.Conn, remoteAddress string, idleTimeout time.Duration) {
	log.Printf("forwarding connection from client %s to target %s", client.RemoteAddr(), remoteAddress)

	defer client.Close()

	target, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)
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

	// create an channel that stores the results from copy operations
	done := make(chan copyResult, 2)

	//initialize deadline
	deadline := &idleDeadline{
		timeout: idleTimeout,
		client: client,
		target: target,
	}

	// wrap client and target in new timeout structs
	clientWithTimeout := &idleTimeoutConn{
		conn: client,
		deadline: deadline,
	}

	targetWithTimeout := &idleTimeoutConn{
		conn: target,
		deadline: deadline,
	}

	// STATS
	start := time.Now()

	// Client request traffic:
	// client -> target
	go copyConnection(done, targetWithTimeout, clientWithTimeout)

	// Target response traffic:
	// target -> client
	go copyConnection(done, clientWithTimeout, targetWithTimeout)

	first := <-done
	
	// Closing both connections wakes up the remaining io.Copy goroutine in case either client or target disconnects mid transfer
	_ = client.Close()
	_ = target.Close()

	second := <-done

	duration := time.Since(start)

	// ------------- PRINTING RESULTS -------------
	if isTimeout(first.err) || isTimeout(second.err) {
		log.Printf("closed idle connection: %s after %s", client.RemoteAddr().String(), remoteAddress)
	} else {
		log.Printf("connection closed: %s → %s", client.RemoteAddr().String() ,remoteAddress)
	}
	log.Printf("transferred: sent=%d bytes received=%d bytes duration: %d ms", first.bytes, second.bytes, duration.Milliseconds())
}

func copyConnection(done chan<- copyResult, destination io.Writer, source io.Reader) {
	bytesCopied, err := io.Copy(destination, source)

	done <- copyResult{
		bytes: bytesCopied,
		err: err,
	}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error // net.Error is an interface for network-related errors
	return errors.As(err, &netErr) && netErr.Timeout() // if the error is a net.Error store it in netErr, check if the error is a timeout error
}
