# Phase 1: One-Port Forwarder MVP

Portly is a simple TCP port forwarder that sits between a client and a
destination server:

```text
Client                  Forwarder                   Destination
curl/browser            Portly                      web server
     |                     |                            |
     |-- connect :8080 --->|                            |
     |                     |-- connect :9000 ---------->|
     |-- request bytes --->|-- request bytes ---------->|
     |<-- response bytes --|<-- response bytes ---------|
```

The forwarder maintains two separate TCP connections:

1. Client to forwarder
2. Forwarder to destination

## Open a local listener

```go
listener, err := net.Listen("tcp", ":8080")
```

This asks the operating system to accept TCP connections on local port 8080.
The returned `net.Listener` acts like a reception desk: it waits for clients
rather than representing one specific connection.

## Accept clients

```go
client, err := listener.Accept()
```

`Accept` waits for a client and returns a `net.Conn` for that connection. The
listener remains open and can continue accepting other clients:

```text
listener
├── waits on port 8080
├── client connection 1
├── client connection 2
└── client connection 3
```

## Connect to the destination

```go
target, err := net.Dial("tcp", "127.0.0.1:9000")
```

`Dial` is the client-side counterpart to `Listen`:

- `Listen` waits for incoming connections.
- `Dial` opens an outgoing connection.

Both `client` and `target` are `net.Conn` values. A connection can read and
write data, expose its local and remote addresses, and be closed.

## Forward bytes in both directions

```go
io.Copy(destination, source)
```

`io.Copy` reads from the source and writes to the destination until the source
closes or an error occurs. Portly needs to copy traffic in both directions:

```text
client ----------> target
client <---------- target
```

The copies run concurrently because TCP traffic can flow independently in both
directions. Each direction reports its outcome as a single `copyResult`
(byte count, error, and which direction it was) on a shared channel, so the
handler doesn't need to correlate separate byte/error channels per direction:

```go
type copyResult struct {
	bytes    int64
	err      error
	sentFlag bool
}

func copyConnection(done chan<- copyResult, destination io.Writer, source io.Reader, sent bool) {
	bytesCopied, err := io.Copy(destination, source)
	done <- copyResult{bytes: bytesCopied, err: err, sentFlag: sent}
}

done := make(chan copyResult, 2)
go copyConnection(done, target, client, true)
go copyConnection(done, client, target, false)

first := <-done
_ = client.Close()
_ = target.Close()
second := <-done
```

The handler waits for the first direction to finish, then closes both
connections to unblock whichever `io.Copy` is still running (e.g. the other
side hasn't sent an EOF), then waits for its result too. `sentFlag` tells the
handler which result is "sent" vs "received" regardless of which one arrives
first, avoiding a bug where byte counts were previously misattributed based on
channel arrival order.

## Idle timeouts

Long-lived but silent connections (e.g. a client that connects and never sends
anything) would otherwise be held open forever. Portly closes a connection
after `-idle-timeout` (default `5m`, `0` disables) has passed with no traffic
in either direction.

An `idleDeadline` holds the shared timeout and refreshes both the client and
target connection's deadline together, since either side going idle should
close the whole forwarded connection:

```go
type idleDeadline struct {
	timeout time.Duration
	client  net.Conn
	target  net.Conn
}

func (d *idleDeadline) refresh() error {
	deadline := time.Now().Add(d.timeout)
	if err := d.client.SetDeadline(deadline); err != nil {
		return err
	}
	return d.target.SetDeadline(deadline)
}
```

`idleTimeoutConn` wraps a `net.Conn` so every `Read`/`Write` refreshes the
deadline before touching the underlying connection. `client` and `target` are
each wrapped before being passed into `copyConnection`, so traffic in either
direction keeps the connection alive:

```go
type idleTimeoutConn struct {
	conn     net.Conn
	deadline *idleDeadline
}

func (c *idleTimeoutConn) Read(buffer []byte) (int, error) {
	if err := c.deadline.refresh(); err != nil {
		return 0, err
	}
	return c.conn.Read(buffer)
}
```

When a copy fails because the deadline was exceeded, `isTimeout` distinguishes
that from a normal disconnect so the log line reads "closed idle connection"
instead of "connection closed":

```go
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
```

## Testing

Run the test suite with:

```bash
go test ./...
```
