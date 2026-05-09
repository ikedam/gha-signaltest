package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: signaltracer <command> [args...]")
		os.Exit(2)
	}

	name := os.Args[1]
	args := os.Args[2:]

	sigCh := make(chan os.Signal, 8)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "signaltracer: start: %v\n", err)
		os.Exit(1)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case sig := <-sigCh:
			fmt.Printf("signaltracer: received %v at %s pid=%d\n", sig, time.Now().Format(time.RFC3339Nano), os.Getpid())
		case err := <-done:
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if st, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						os.Exit(st.ExitStatus())
					}
				}
				fmt.Fprintf(os.Stderr, "signaltracer: wait: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
}
