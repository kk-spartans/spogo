//go:build !windows

package main

import (
	"io"
}

func shouldRunDaemonServer(args []string) bool { return false }

func proxyToDaemon(args []string, out io.Writer, errOut io.Writer) (int, bool) { return 0, false }
