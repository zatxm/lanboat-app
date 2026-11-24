package desktop

import (
	"fmt"
	"github.com/goccy/go-json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zatxm/lumberjack/v26"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}
type Logs struct {
	file    *lumberjack.Logger
	buffer  *zapcore.BufferedWriteSyncer
	mu      sync.Mutex
	entries []LogEntry
}

// Write receives each encoded JSON entry. The in-memory copy keeps the fields
// (errors, counts) after the message so the UI shows the whole event.
func (l *Logs) Write(b []byte) (int, error) {
	var item map[string]any
	if json.Unmarshal(b, &item) != nil {
		return len(b), nil
	}
	entry := LogEntry{}
	entry.Time, _ = item["ts"].(string)
	entry.Level, _ = item["level"].(string)
	entry.Message, _ = item["msg"].(string)
	keys := make([]string, 0, len(item))
	for key := range item {
		switch key {
		case "ts", "level", "msg", "caller", "stacktrace", "logger":
		default:
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		entry.Message += " " + key + "=" + fmt.Sprint(item[key])
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) >= 200 {
		copy(l.entries, l.entries[1:])
		l.entries = l.entries[:199]
	}
	l.entries = append(l.entries, entry)
	return len(b), nil
}

func (l *Logs) List() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]LogEntry{}, l.entries...)
}

func NewLogger(dir string) (*zap.Logger, *Logs, func() error) {
	logs := &Logs{entries: []LogEntry{}}
	file := &lumberjack.Logger{Filename: filepath.Join(dir, "logs", "app.log"), MaxSize: 2, MaxBackups: 2, MaxAge: 0}
	// Batch file writes while keeping the in-memory list for the UI immediate.
	buffered := &zapcore.BufferedWriteSyncer{WS: zapcore.AddSync(file), Size: 256 << 10, FlushInterval: time.Second}
	logs.file, logs.buffer = file, buffered
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	logger := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(config), zapcore.NewMultiWriteSyncer(buffered, zapcore.AddSync(logs)), zap.InfoLevel))
	return logger, logs, func() error { _ = buffered.Stop(); return file.Close() }
}

func (l *Logs) Clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		if l.buffer != nil {
			// Write pending entries before rotating them away.
			_ = l.buffer.Sync()
		}
		if err := l.file.Rotate(); err != nil {
			return err
		}
		dir := filepath.Dir(l.file.Filename)
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "app-") {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
	l.entries = nil
	return nil
}
