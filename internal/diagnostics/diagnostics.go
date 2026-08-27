package diagnostics

import (
	"fmt"
	"net"
	"time"
)

type DiagnosticResult struct {
	Check string
	Success bool
	Message string
}

// checks if target is available for portly to open
func checkTarget(remoteAddress string) DiagnosticResult {
	target, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)

	if err != nil {
		return DiagnosticResult{
			Check: "target",
			Success: false,
			Message: fmt.Sprintf("cannot reach %s: %v", remoteAddress, err),
		}
	}

	defer target.Close()

	return DiagnosticResult{
			Check: "target",
			Success: true,
			Message: fmt.Sprintf("%s is reachable", target),
		}
}

// checks if listener can open on listen address
func checkListenAddress(address string) DiagnosticResult {
	listener, err := net.Listen("tcp", address)

	if err != nil {
		return DiagnosticResult{
			Check: "listener",
			Success: false,
			Message: fmt.Sprintf("cannot listen on %s: %v", address, err),
		}
	}

	defer listener.Close()

	return DiagnosticResult{
			Check: "listener",
			Success: true,
			Message: fmt.Sprintf("%s is available", listener.Addr()),
	}
}

func checkExposure(address string) DiagnosticResult {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return DiagnosticResult{
			Check:   "exposure",
			Success: false,
			Message: fmt.Sprintf("invalid listen address: %v", err),
		}
	}

	switch host {
	case "127.0.0.1", "localhost", "::1":
		return DiagnosticResult{
			Check: "exposure",
			Success: true,
			Message: "listener is restricted to this machine",
		}

	case "0.0.0.0", "::":
		return DiagnosticResult{
			Check: "exposure",
			Success: true,
			Message: "warning: listener accepts connections on all network interfaces",
		}

	default:
		return DiagnosticResult{
			Check: "exposure",
			Success: true,
			Message: fmt.Sprintf("listener is bound to %s", host),
		}
	}
}