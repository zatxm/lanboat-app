//go:build !windows

package appmeta

func installedDataDir() (string, error) { return "", nil }
