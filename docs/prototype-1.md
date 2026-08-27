# Portly Internals

Portly is a TCP port forwarder that sits between clients and one or more
destination servers, one pair of listener/target per configured rule:

```text
Client                  Forwarder                   Destination
curl/browser            Portly                      web server
     |                     |                            |
     |-- connect :8080 --->|                            |
     |                     |-- connect :3000 ---------->|
     |-- request bytes --->|-- request bytes ---------->|
     |<-- response bytes --|<-- response bytes ---------|
```

For each rule, the forwarder maintains two separate TCP connections:

1. Client to forwarder
2. Forwarder to destination

## Loading rules from the config file

`main.go` parses a single `-config` flag (default `portly.yaml`) and hands the
path to `config.Load`:

```go
configPath := flag.String("config", "portly.yaml", "path to the Portly config file")
flag.Parse()

rules, err := config.Load(*configPath)
```

`config.Load` reads the YAML file into a `File{ Rules []Rule }`, then
validates and converts each `Rule` into a `RuntimeRule`:

```go
type Rule struct {
	Name        string `yaml:"name"`
	Listen      string `yaml:"listen"`
	Target      string `yaml:"target"`
	IdleTimeout string `yaml:"idle_timeout"`
}

type RuntimeRule struct {
	Name        string
	Listen      string
	Target      string
	IdleTimeout time.Duration
}
```

Validation rejects a config with no rules, a missing `name`/`listen`/`target`,
a `listen` equal to `target`, a duplicate `name` or `listen` address across
rules, or an unparseable/negative `idle_timeout`. `idle_timeout` defaults to
`5m` when omitted; `0` disables idle timeouts for that rule. See
`internal/config/config.go`.

## One Forwarder per rule

Each `RuntimeRule` becomes its own `*forwarder.Forwarder`, so a single Portly
process can run many independent listener/target pairs at once:

```go
forwarders := make([]*forwarder.Forwarder, 0, len(rules))
for _, rule := range rules {
	forwarders = append(forwarders, forwarder.New(rule))
}

for _, f := range forwarders {
	forwarderProcesses.Add(1)
	go func(f *forwarder.Forwarder) {
		defer forwarderProcesses.Done()
		if err := f.Start(ctx); err != nil {
			errs <- err
			stop()
		}
	}(f)
}
```

If any rule fails to start (e.g. its `listen` address is already in use), that
error cancels the shared context, which stops every other rule's listener
too.

## Open a local listener

Inside `Forwarder.Start`, each rule opens its own listener on the address from
its config:

```go
listener, err := net.Listen("tcp", f.rule.Listen)
```

The returned `net.Listener` acts like a reception desk: it waits for clients
rather than representing one specific connection. A goroutine watches the
shared context and closes this rule's listener as soon as shutdown is
requested:

```go
go func() {
	<-ctx.Done()
	listener.Close()
}()
```

## Accept clients

`Forwarder.acceptConnections` loops on `Accept`, handing each client off to
its own goroutine so the listener can keep accepting further clients for that
rule:

```go
client, err := f.listener.Accept()
```

```text
listener (rule "web")
├── waits on 127.0.0.1:8080
├── client connection 1
├── client connection 2
└── client connection 3
```

`f.connections` (a `sync.WaitGroup`) tracks in-flight connections per rule so
shutdown can wait for them to finish. See "Graceful shutdown" below.

## Connect to the destination

`handlePortForward` (in `internal/forwarder/connection.go`) is the
client-side counterpart to `Listen`:

```go
target, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)
```

- `Listen` waits for incoming connections.
- `Dial` opens an outgoing connection.

Both `client` and `target` are `net.Conn` values. A connection can read and
write data, expose its local and remote addresses, and be closed.

## Forward bytes in both directions

```go
io.Copy(destination, source)
```

`io.Copy` reads from the source and writes to the destination until the source
closes or an error occurs. Each forwarded connection copies traffic in both
directions:

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
after that rule's `idle_timeout` (from the config, default `5m`, `0`
disables) has passed with no traffic in either direction.

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
	if d.timeout <= 0 {
		return nil
	}
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

## Graceful shutdown

On `Ctrl+C`/`SIGTERM`, `main.go` cancels the shared context. Each
`Forwarder`'s watcher goroutine (above) closes that rule's listener, which
makes its blocked `Accept` return `net.ErrClosed` and stops
`acceptConnections` without treating it as an error.

After every forwarder's listener has stopped, `main.go` calls `Wait` on each
`Forwarder` in turn. `Wait` gives that rule's still-active connections up to
10 seconds to finish on their own:

```go
select {
case <-done: // f.connections.Wait() completed
	return
case <-time.After(10 * time.Second):
	// timeout exceeded
}

close(f.shutdown)
<-done
```

If the timeout elapses, `Wait` closes the rule's `shutdown` channel. Each
in-flight connection goroutine is racing on that channel and force-closes its
client connection as soon as it fires, which unblocks the `io.Copy` calls in
`handlePortForward` so the connection can finish closing instead of hanging
forever.

## Testing

Run the test suite with:

```bash
go test ./...
```
