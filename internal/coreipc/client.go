package coreipc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	json "github.com/goccy/go-json"
	"github.com/zatxm/lanboat-app/internal/traffic"
)

type Logging struct{ Proxy, Request, DNS bool }

type Request struct {
	ID       uint64
	Target   string
	Deadline int64
	Op       string
	Config   json.RawMessage
	TUN      bool
	Logging  Logging
}

type Stats struct{ Uplink, Downlink, Connections int64 }

type Response struct {
	ID         uint64         `json:",omitempty"`
	Latency    int64          `json:",omitempty"`
	Controller string         `json:",omitempty"`
	Error      string         `json:",omitempty"`
	Traffic    *traffic.Entry `json:",omitempty"`
	Stats      *Stats         `json:",omitempty"`
}

type Session interface {
	PID() int
	ControllerAddress() string
	Done() <-chan struct{}
	Err() error
	Call(context.Context, Request) (Response, error)
	Close()
}

type Launcher func(context.Context, Request, func(traffic.Entry)) (Session, error)
type Backend func(context.Context, io.Reader, io.Writer) error

// SetEmbeddedBackend is called during mobile startup, before using the engine.
func SetEmbeddedBackend(backend Backend) { embedded = backend }

var embedded Backend

type Client struct {
	pid        int
	abort      func()
	input      io.WriteCloser
	readerDone chan struct{}
	done       chan struct{}
	err        error
	pending    sync.Map
	sequence   atomic.Uint64
	controller string
	mu         sync.Mutex
	marshal    *json.Encoder
	closeOnce  sync.Once
}

func Executable(app string) (string, error) {
	if embedded != nil {
		return "lanboat-embedded", nil
	}
	if os.Getenv("LANBOAT_CORE_TEST") == "1" {
		if path := os.Getenv("LANBOAT_CORE_TEST_BINARY"); path != "" {
			return path, nil
		}
		return os.Executable()
	}
	name := "lanboat-core"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	own, _ := os.Executable()
	dirs := []string{filepath.Dir(app), filepath.Dir(own)}
	if runtime.GOOS == "linux" {
		dirs = append(dirs, "/usr/libexec")
	}
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			continue
		}
		path := filepath.Join(dir, name)
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", errors.New("找不到 Lanboat 网络后端，请安装完整的安装包")
}

func Launch(ctx context.Context, path string, request Request, trafficLog func(traffic.Entry)) (*Client, error) {
	if embedded != nil {
		return LaunchEmbedded(ctx, request, trafficLog, embedded)
	}
	cmd := exec.Command(path, "--lanboat-core")
	hideWindow(cmd)
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	return connect(ctx, request, trafficLog, input, output, cmd.Process.Pid, cmd.Wait, func() { _ = cmd.Process.Kill() })
}

func LaunchEmbedded(ctx context.Context, request Request, trafficLog func(traffic.Entry), backend Backend) (*Client, error) {
	fromApp, toCore := io.Pipe()
	fromCore, toApp := io.Pipe()
	run, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() {
		err := backend(run, fromApp, toApp)
		cancel()
		fromApp.Close()
		toApp.Close()
		stopped <- err
	}()
	abort := func() {
		cancel()
		fromApp.Close()
		fromCore.Close()
	}
	return connect(ctx, request, trafficLog, toCore, fromCore, os.Getpid(), func() error { return <-stopped }, abort)
}

func connect(ctx context.Context, request Request, trafficLog func(traffic.Entry), input io.WriteCloser, output io.Reader, pid int, wait func() error, abort func()) (*Client, error) {
	c := &Client{pid: pid, abort: abort, input: input, readerDone: make(chan struct{}), done: make(chan struct{}), marshal: json.NewEncoder(input)}
	go func() {
		defer close(c.readerDone)
		decoder := json.NewDecoder(output)
		for {
			var response Response
			if err := decoder.Decode(&response); err != nil {
				return
			}
			if response.Traffic != nil {
				if trafficLog != nil {
					trafficLog(*response.Traffic)
				}
			} else {
				if pending, ok := c.pending.Load(response.ID); ok {
					pending.(chan Response) <- response
				}
			}
		}
	}()
	go func() {
		<-c.readerDone
		c.err = wait()
		close(c.done)
	}()
	interruptStartup := context.AfterFunc(ctx, func() { input.Close() })
	response, err := c.Call(ctx, request)
	interruptStartup()
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	c.controller = response.Controller
	return c, nil
}

func (c *Client) PID() int { return c.pid }

func (c *Client) ControllerAddress() string { return c.controller }
func (c *Client) Done() <-chan struct{}     { return c.done }
func (c *Client) Err() error                { return c.err } // Call only after Done closes.

func (c *Client) Call(ctx context.Context, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	request.ID = c.sequence.Add(1)
	if deadline, ok := ctx.Deadline(); ok {
		request.Deadline = deadline.UnixNano()
	}
	reply := make(chan Response, 1)
	c.pending.Store(request.ID, reply)
	defer c.pending.Delete(request.ID)
	c.mu.Lock()
	err := c.marshal.Encode(request)
	c.mu.Unlock()
	if err != nil {
		return Response{}, fmt.Errorf("发送网络后端指令失败：%w", err)
	}
	var response Response
	select {
	case response = <-reply:
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-c.done:
		// A backend can send an error and then exit.
		select {
		case response = <-reply:
		default:
			return Response{}, errors.New("网络后端已退出")
		}
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.input.Close()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-c.done:
		case <-timer.C:
			c.abort()
			<-c.done
		}
	})
}

func RunCommand(ctx context.Context, path string, request Request) (Response, error) {
	if deadline, ok := ctx.Deadline(); ok {
		request.Deadline = deadline.UnixNano()
	}
	config, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}
	var output []byte
	var runErr error
	if embedded != nil {
		var reply bytes.Buffer
		runErr = embedded(ctx, bytes.NewReader(config), &reply)
		output = reply.Bytes()
	} else {
		cmd := exec.CommandContext(ctx, path, "--lanboat-core")
		hideWindow(cmd)
		cmd.Stdin = bytes.NewReader(config)
		output, runErr = cmd.Output()
	}
	var response Response
	if err = json.Unmarshal(output, &response); err != nil {
		if runErr != nil {
			return Response{}, runErr
		}
		return Response{}, err
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, runErr
}
