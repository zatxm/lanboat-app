package engine

import (
	"io"
	"os"
	"path/filepath"
)

// The diagnostic must have a distinct executable path from the TUN-bypassed GUI.
func prepareDNSProbe(executable, dir string) (string, func(), error) {
	temporary, err := os.MkdirTemp(dir, ".dns-probe-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(temporary) }
	path := filepath.Join(temporary, "probe")
	if err = os.Link(executable, path); err == nil {
		return path, cleanup, nil
	}
	source, err := os.Open(executable)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	_, err = io.Copy(target, source)
	closeErr := target.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}
