package desktop

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Memory struct {
	RSS       int64 `json:"rss"`
	Processes int   `json:"processes"`
	Limit     int64 `json:"limit"`
}

func ProcessMemory(root int) Memory {
	result := Memory{Limit: 200_000_000}
	pending := []int{root}
	seen := map[int]bool{}
	for len(pending) > 0 {
		pid := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		dir := filepath.Join("/proc", strconv.Itoa(pid))
		data, err := os.ReadFile(filepath.Join(dir, "status"))
		if err != nil {
			continue
		}
		result.RSS += residentBytes(string(data))
		result.Processes++
		// Go can start a helper from a non-leader thread.
		tasks, _ := os.ReadDir(filepath.Join(dir, "task"))
		for _, task := range tasks {
			children, err := os.ReadFile(filepath.Join(dir, "task", task.Name(), "children"))
			if err != nil {
				continue
			}
			for child := range strings.FieldsSeq(string(children)) {
				pid, _ := strconv.Atoi(child)
				pending = append(pending, pid)
			}
		}
	}
	return result
}

func residentBytes(status string) int64 {
	for line := range strings.SplitSeq(status, "\n") {
		if value, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			for kib := range strings.FieldsSeq(value) {
				n, _ := strconv.ParseInt(kib, 10, 64)
				return n * 1024
			}
			break
		}
	}
	return 0
}
