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
directions:

```go
go io.Copy(target, client)
go io.Copy(client, target)
```

A separate buffered channel records the byte count and error from each copy
direction:

```go
errClientToTarget := make(chan error, 1)
errTargetToClient := make(chan error, 1)
sent := make(chan int, 1)
recieved := make(chan int, 1)
start := time.Now()

go func() {
	bytes, err := io.Copy(target, client)
	sent <- int(bytes)
	errClientToTarget <- err
}()

go func() {
	bytes, err := io.Copy(client, target)
	recieved <- int(bytes)
	errTargetToClient <- err
}()

sentBytes := <-sent
receivedBytes := <-recieved
clientToTargetErr := <-errClientToTarget
targetToClientErr := <-errTargetToClient
duration := time.Since(start)
```

The handler waits for both directions to finish, logs either direction's error,
and reports the sent bytes, received bytes, and connection duration.

## Testing

Run the test suite with:

```bash
go test ./...
```
