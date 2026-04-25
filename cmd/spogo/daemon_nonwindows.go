//go:build !windows

package main

import (
	"fmt"
	"io"
)

func shouldRunDaemonServer(args []string) bool { return false }

func runDaemonServe(args []string, out io.Writer, errOut io.Writer) int {
	_, _ = fmt.Fprintln(errOut, "internal daemon server mode is only supported on Windows")
	return 1
}

func proxyToDaemon(args []string, out io.Writer, errOut io.Writer) (int, bool) { return 0, false }
