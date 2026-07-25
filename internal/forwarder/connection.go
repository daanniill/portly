package forwarder

import (
	"errors"
	"io"
	"log"
	"net"
	"time"
)

type idleDeadline struct {
	timeout time.Duration
	client  net.Conn
	target  net.Conn
}

func (d *idleDeadline) refresh() error {
	if d.timeout <= 0 {
		return nil
	}

	deadline := time.Now().Add(d.timeout)

	if err := d.client.SetDeadline(deadline); err != nil {
		return err
	}

	return d.target.SetDeadline(deadline)
}

// create a new struct that wraps conn objects and overwrites the read and write methods of net.Conn objects
// idleTimeoutConn becomes a wrapper around a real network connection that automatically refreshes the idle timeout whenever data is read or written.
type idleTimeoutConn struct {
	conn     net.Conn
	deadline *idleDeadline
}

// read but with refresh
func (c *idleTimeoutConn) Read(buffer []byte) (int, error) {
	if err := c.deadline.refresh(); err != nil {
		return 0, err
	}

	return c.conn.Read(buffer)
}

//write but with refresh
func (c *idleTimeoutConn) Write(buffer []byte) (int, error) {
	if err := c.deadline.refresh(); err != nil {
		return 0, err
	}

	return c.conn.Write(buffer)
}

type copyResult struct {
	bytes    int64
	err      error
	sentFlag bool
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
		client:  client,
		target:  target,
	}

	// wrap client and target in new timeout structs
	clientWithTimeout := &idleTimeoutConn{
		conn:     client,
		deadline: deadline,
	}

	targetWithTimeout := &idleTimeoutConn{
		conn:     target,
		deadline: deadline,
	}

	// STATS
	start := time.Now()

	// Client request traffic:
	// client -> target
	go copyConnection(done, targetWithTimeout, clientWithTimeout, true)

	// Target response traffic:
	// target -> client
	go copyConnection(done, clientWithTimeout, targetWithTimeout, false)

	first := <-done

	// Closing both connections wakes up the remaining io.Copy goroutine in case either client or target disconnects mid transfer
	_ = client.Close()
	_ = target.Close()

	second := <-done

	duration := time.Since(start)

	// ------------- PRINTING RESULTS -------------
	if isTimeout(first.err) || isTimeout(second.err) {
		log.Printf("closed idle connection: %s → %s after %s", client.RemoteAddr().String(), remoteAddress, duration.Round(time.Millisecond))
	} else {
		log.Printf("connection closed: %s → %s", client.RemoteAddr().String(), remoteAddress)
	}

	if first.sentFlag {
		log.Printf("transferred: sent=%d bytes received=%d bytes duration: %d ms", first.bytes, second.bytes, duration.Milliseconds())
	} else {
		log.Printf("transferred: sent=%d bytes received=%d bytes duration: %d ms", second.bytes, first.bytes, duration.Milliseconds())
	}
}

func copyConnection(done chan<- copyResult, destination io.Writer, source io.Reader, sent bool) {
	bytesCopied, err := io.Copy(destination, source)

	done <- copyResult{
		bytes:    bytesCopied,
		err:      err,
		sentFlag: sent,
	}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error                               // net.Error is an interface for network-related errors
	return errors.As(err, &netErr) && netErr.Timeout() // if the error is a net.Error store it in netErr, check if the error is a timeout error
}



