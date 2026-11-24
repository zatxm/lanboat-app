package mobilevpn

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

func TestPacketBoundariesAndClose(t *testing.T) {
	var sent []byte
	device := NewPackets(func(packet []byte) bool {
		sent = bytes.Clone(packet)
		return true
	})
	device.Push([]byte{0x45, 0x01})
	device.Push([]byte{0x60, 0x02})
	buffer := make([]byte, 10)
	for _, header := range []byte{0x45, 0x60} {
		if n, err := device.Read(buffer); n != 2 || err != nil || buffer[0] != header {
			t.Fatalf("packet boundary lost: %d %v %x", n, err, buffer)
		}
	}
	if n, err := device.Write(buffer[:2]); n != 2 || err != nil || !bytes.Equal(sent, buffer[:2]) {
		t.Fatalf("packet write failed: %d %v %x", n, err, sent)
	}
	read := make(chan error, 1)
	go func() { _, err := device.Read(buffer); read <- err }()
	_ = device.Close()
	select {
	case err := <-read:
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("closing the packet adapter did not unblock its reader")
	}
	if device.Push([]byte{0x45}) {
		t.Fatal("closed packet adapter accepted traffic")
	}
}
