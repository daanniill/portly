package main

import (
	"fmt"
)

var (
	localAddress string = "localhost:8081"
	remoteForwarder string = "https://example.com"
)

func main() {
	fmt.Println("Start portly")

	// Handle panics raised from the server

	defer func() {
		if e := recover(); e != nil { // creates a variable e := recover() (returns value cause by panic), then checks if its not nil
			fmt.Printf(
				"[CRITICAL] encountered a critical error, recovering from panic, error trace: %v",
				e,
			)
		}
	}() // defines anonymous function
}