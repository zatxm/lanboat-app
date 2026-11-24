//go:build ios && cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
void lbIOSStart(int64_t id, const void *data, int len);
void lbIOSStop(int64_t id);
void lbIOSMessage(int64_t id, int64_t ticket, const void *data, int len);
int lbIOSOpen(const char *url);
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"

	json "github.com/goccy/go-json"
	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/traffic"
)

type iosReply struct {
	Response coreipc.Response
	Logs     []traffic.Entry
}

type iosSession struct {
	id     int64
	ready  chan struct{}
	done   chan struct{}
	closed sync.Once
	err    error
	log    func(traffic.Entry)
}

var iosID atomic.Int64
var iosSessions, iosMessages sync.Map

//export LanboatIOSEvent
func LanboatIOSEvent(id C.int64_t, state C.int, message *C.char) {
	value, ok := iosSessions.Load(int64(id))
	if !ok {
		return
	}
	s := value.(*iosSession)
	switch state {
	case 1:
		select {
		case s.ready <- struct{}{}:
		default:
		}
	case 0:
		s.end(nil)
	default:
		s.end(errors.New(C.GoString(message)))
	}
}

//export LanboatIOSReply
func LanboatIOSReply(ticket C.int64_t, data unsafe.Pointer, size C.int) {
	if r, ok := iosMessages.LoadAndDelete(int64(ticket)); ok {
		r.(chan []byte) <- C.GoBytes(data, size)
	}
}

func launchVPN(ctx context.Context, req coreipc.Request, log func(traffic.Entry)) (coreipc.Session, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("iOS VPN 配置不能超过 1 MiB")
	}
	s := &iosSession{id: iosID.Add(1), ready: make(chan struct{}, 1), done: make(chan struct{}), log: log}
	iosSessions.Store(s.id, s)
	C.lbIOSStart(C.int64_t(s.id), unsafe.Pointer(unsafe.SliceData(raw)), C.int(len(raw)))
	select {
	case <-ctx.Done():
		s.Close()
		return nil, ctx.Err()
	case <-s.done:
		if s.err == nil {
			return nil, errors.New("iOS VPN 在启动期间断开")
		}
		return nil, s.err
	case <-s.ready:
		return s, nil
	}
}

func (s *iosSession) end(err error) {
	s.closed.Do(func() {
		s.err = err
		close(s.done)
		iosSessions.Delete(s.id)
	})
}

func (s *iosSession) Close() {
	s.end(nil)
	C.lbIOSStop(C.int64_t(s.id))
}
func (s *iosSession) Done() <-chan struct{}     { return s.done }
func (s *iosSession) Err() error                { return s.err }
func (s *iosSession) PID() int                  { return 0 }
func (s *iosSession) ControllerAddress() string { return "" }

func (s *iosSession) Call(ctx context.Context, req coreipc.Request) (coreipc.Response, error) {
	empty := coreipc.Response{}
	if deadline, ok := ctx.Deadline(); ok {
		req.Deadline = deadline.UnixNano()
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return empty, err
	}
	ticket := iosID.Add(1)
	reply := make(chan []byte, 1)
	iosMessages.Store(ticket, reply)
	defer iosMessages.Delete(ticket)
	C.lbIOSMessage(C.int64_t(s.id), C.int64_t(ticket), unsafe.Pointer(unsafe.SliceData(raw)), C.int(len(raw)))
	select {
	case <-ctx.Done():
		return empty, ctx.Err()
	case <-s.done:
		return empty, errors.New("iOS VPN 已断开")
	case data := <-reply:
		var result iosReply
		if err := json.Unmarshal(data, &result); err != nil {
			return empty, errors.New("iOS VPN 扩展未响应")
		}
		if result.Response.Error != "" {
			return empty, errors.New(result.Response.Error)
		}
		if s.log != nil {
			for _, log := range result.Logs {
				s.log(log)
			}
		}
		return result.Response, nil
	}
}

func openExternal(url string) error {
	value := C.CString(url)
	defer C.free(unsafe.Pointer(value))
	if C.lbIOSOpen(value) == 0 {
		return errors.New("无法打开链接")
	}
	return nil
}
