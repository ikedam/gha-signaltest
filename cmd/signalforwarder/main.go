package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	sigs := []syscall.Signal{syscall.SIGINT, syscall.SIGTERM}
	if env := os.Getenv("FORWARDSIGNAL"); env != "" {
		var err error
		sigs, err = parseSignals(env)
		if err != nil {
			fmt.Fprintf(os.Stderr, "signalforwarder: FORWARDSIGNAL: %v\n", err)
			os.Exit(2)
		}
	}

	cmd := exec.Command("bash", os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	var forwarded sync.Map
	sigCh := make(chan os.Signal, 8)
	var notify []os.Signal
	for _, s := range sigs {
		notify = append(notify, s)
	}
	signal.Notify(sigCh, notify...)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "signalforwarder: start bash: %v\n", err)
		os.Exit(1)
	}

	childPID := cmd.Process.Pid
	childPGID, err := syscall.Getpgid(childPID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "signalforwarder: getpgid: %v\n", err)
		_ = cmd.Process.Kill()
		os.Exit(1)
	}

	setForegroundPGID(childPGID)

	fmt.Printf("signalforwarder: started bash pid=%d pgid=%d, self pid=%d\n", childPID, childPGID, os.Getpid())

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	for {
		select {
		case sig := <-sigCh:
			ss := sig.(syscall.Signal)
			if _, dup := forwarded.LoadOrStore(ss, struct{}{}); dup {
				fmt.Printf("signalforwarder: received %v at %s pid=%d (already forwarded, skipping)\n",
					sig, time.Now().Format(time.RFC3339Nano), os.Getpid())
				continue
			}

			pgid, psrc := foregroundPGID(childPGID)
			fmt.Printf("signalforwarder: received %v at %s pid=%d forwarding to pgid=%d (%s)\n",
				sig, time.Now().Format(time.RFC3339Nano), os.Getpid(), pgid, psrc)

			if err := syscall.Kill(-pgid, ss); err != nil {
				fmt.Fprintf(os.Stderr, "signalforwarder: kill(-%d, %v): %v\n", pgid, ss, err)
			}

		case err := <-done:
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if st, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						os.Exit(st.ExitStatus())
					}
				}
				fmt.Fprintf(os.Stderr, "signalforwarder: wait: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
}

func setForegroundPGID(pgid int) {
	for _, fd := range []int{int(os.Stdin.Fd()), int(os.Stdout.Fd()), int(os.Stderr.Fd())} {
		if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pgid); err == nil {
			fmt.Printf("signalforwarder: tcsetpgrp fd=%d pgid=%d\n", fd, pgid)
			return
		}
	}
	fmt.Printf("signalforwarder: tcsetpgrp unavailable (no tty), will forward to child pgid=%d\n", pgid)
}

func foregroundPGID(fallbackChildPGID int) (pgid int, source string) {
	for _, fd := range []int{int(os.Stdin.Fd()), int(os.Stdout.Fd()), int(os.Stderr.Fd())} {
		p, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
		if err == nil && p > 0 {
			return p, "TIOCGPGRP"
		}
	}
	return fallbackChildPGID, "child_pgid"
}

func parseSignals(s string) ([]syscall.Signal, error) {
	parts := strings.Split(s, ",")
	var out []syscall.Signal
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		sig, err := oneSignal(p)
		if err != nil {
			return nil, err
		}
		out = append(out, sig)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no signals in %q", s)
	}
	return out, nil
}

func oneSignal(name string) (syscall.Signal, error) {
	if !strings.HasPrefix(name, "SIG") {
		name = "SIG" + name
	}
	switch name {
	case "SIGINT":
		return syscall.SIGINT, nil
	case "SIGTERM":
		return syscall.SIGTERM, nil
	case "SIGHUP":
		return syscall.SIGHUP, nil
	case "SIGQUIT":
		return syscall.SIGQUIT, nil
	case "SIGUSR1":
		return syscall.SIGUSR1, nil
	case "SIGUSR2":
		return syscall.SIGUSR2, nil
	default:
		if n, err := strconv.Atoi(strings.TrimPrefix(name, "SIG")); err == nil {
			return syscall.Signal(n), nil
		}
		return 0, fmt.Errorf("unknown signal: %s", name)
	}
}
