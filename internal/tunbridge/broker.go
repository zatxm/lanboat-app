package tunbridge

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

type Info struct {
	Name string
	Mark uint32
}

type Device struct {
	io.ReadWriteCloser
	Info    Info
	Protect func(uintptr) error
	Done    <-chan struct{}
}

func writeFrame(c io.Writer, data []byte) error {
	if len(data) == 0 || len(data) > 65575 {
		return errors.New("invalid TUN broker frame length")
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := c.Write(hdr[:]); err != nil {
		return err
	}
	n, err := c.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func readFrame(c io.Reader, buf []byte) (int, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint32(hdr[:]))
	if n < 1 || n > len(buf) {
		return 0, errors.New("oversized TUN broker frame")
	}
	return io.ReadFull(c, buf[:n])
}

func sendInfo(c net.Conn, info Info, err error) error {
	if err != nil {
		return writeFrame(c, append([]byte{1}, []byte(err.Error())...))
	}
	b := binary.BigEndian.AppendUint32([]byte{0}, info.Mark)
	return writeFrame(c, append(b, info.Name...))
}

func receiveInfo(c net.Conn) (Info, error) {
	var b [4096]byte
	n, err := readFrame(c, b[:])
	if err != nil {
		return Info{}, err
	}
	if b[0] == 1 {
		return Info{}, errors.New(string(b[1:n]))
	}
	if b[0] != 0 || n < 6 || n > 261 {
		return Info{}, errors.New("invalid TUN broker response")
	}
	return Info{Name: string(b[5:n]), Mark: binary.BigEndian.Uint32(b[1:5])}, nil
}

func parseUID(s string) (uint32, error) {
	uid, err := strconv.ParseUint(s, 10, 32)
	return uint32(uid), err
}

const startupTimeout = 90 * time.Second
