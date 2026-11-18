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

// Follow children of every thread: Go can start a core from a non-leader thread.
// RSS includes shared pages in each process, matching the app's memory budget.
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
		data, err := os.ReadFile(filepath.Join(dir, "statm"))
		if err != nil {
			continue
		}
		fields := strings.Fields(string(data))
		if len(fields) < 2 {
			continue
		}
		rss, _ := strconv.ParseInt(fields[1], 10, 64)
		result.RSS += rss * int64(os.Getpagesize())
		result.Processes++
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
