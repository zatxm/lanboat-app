//go:build linux || darwin

package tunbridge

import "os"

type hostLease struct {
	file    *os.File
	cleanup func()
}

func (l *hostLease) Close() { l.cleanup() }
