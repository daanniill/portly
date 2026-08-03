package forwarder

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// defining a forwarder type
type Forwarder struct {
	TargetAddress string
	IdleTimeout   time.Duration
}

func (f *Forwarder) Run(listener net.Listener, connections *sync.WaitGroup) error {
	// Handler listening function
	// will accept traffic at the bound port and run a goroutine as a non-blocking action to handle forwarding the request to the remote location
	for { // we want to continuously listen for requests and not immediately end the function execution
		client, err := listener.Accept()
		if err != nil {
			// don't return graceful shutdowns as errors
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("failed to accept connection: %w", err)
		}

		connections.Add(1)
		// Handle the actual forwarding to the remote
		go func() {
			defer connections.Done()
			handlePortForward(client, f.TargetAddress, f.IdleTimeout)
		}()

	}
}
