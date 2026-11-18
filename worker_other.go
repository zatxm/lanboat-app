//go:build windows || darwin

package main

import "errors"

func runTUNWorker() error { return errors.New("the privileged TUN helper is available on Linux") }
