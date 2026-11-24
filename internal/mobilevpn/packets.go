package mobilevpn

import (
	"io"
	"sync"
)

// Packets adapts a system packet callback to one-packet reads by the Go core.
type Packets struct {
	queue chan []byte
	done  chan struct{}
	once  sync.Once
	write func([]byte) bool
}

func NewPackets(write func([]byte) bool) *Packets {
	return &Packets{queue: make(chan []byte, 128), done: make(chan struct{}), write: write}
}

// Push takes ownership of packet. A full queue drops packets instead of blocking
// the system's networking queue.
func (p *Packets) Push(packet []byte) bool {
	if len(packet) == 0 || len(packet) > 65575 {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
	}
	select {
	case <-p.done:
		return false
	case p.queue <- packet:
		return true
	default:
		return false
	}
}

func (p *Packets) Read(out []byte) (int, error) {
	select {
	case <-p.done:
		return 0, io.EOF
	case packet := <-p.queue:
		if len(packet) > len(out) {
			return 0, io.ErrShortBuffer
		}
		return copy(out, packet), nil
	}
}

func (p *Packets) Write(packet []byte) (int, error) {
	select {
	case <-p.done:
		return 0, io.ErrClosedPipe
	default:
	}
	if !p.write(packet) {
		return 0, io.ErrShortWrite
	}
	return len(packet), nil
}

func (p *Packets) Close() error {
	p.once.Do(func() { close(p.done) })
	return nil
}
