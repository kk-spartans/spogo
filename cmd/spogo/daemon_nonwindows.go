//go:build !windows

package main

import (
	"fmt"
	"io"
	"strings"
)

const daemonName = "daemon"

func isDaemonCommand(args []string) bool {
	idx := firstCommandIndex(args)
	if idx < 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(args[idx]), daemonName)
}

func runDaemonCommand(args []string, out io.Writer, errOut io.Writer) int {
	_, _ = fmt.Fprintln(errOut, "daemon mode is currently Windows-only")
	return 2
}

func proxyToDaemon(args []string, out io.Writer, errOut io.Writer) (int, bool) {
	return 0, false
}

func firstCommandIndex(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return i
		}
		hasValue, consumed := consumesGlobalValue(arg)
		if hasValue {
			i += consumed
		}
	}
	return -1
}

func consumesGlobalValue(arg string) (bool, int) {
	if strings.HasPrefix(arg, "--") {
		if strings.Contains(arg, "=") {
			return false, 0
		}
		switch arg {
		case "--config", "--profile", "--timeout", "--market", "--language", "--device", "--engine":
			return true, 1
		default:
			return false, 0
		}
	}
	return false, 0
}
