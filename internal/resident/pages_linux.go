package resident

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// ReleaseExecutablePages discards clean executable mappings after initialization.
// The kernel faults pages back in as the process needs them.
func ReleaseExecutablePages() {
	var stat unix.Stat_t
	if unix.Stat("/proc/self/exe", &stat) != nil {
		return
	}
	maps, err := os.Open("/proc/self/smaps")
	if err != nil {
		return
	}
	defer maps.Close()
	device := fmt.Sprintf("%02x:%02x", unix.Major(uint64(stat.Dev)), unix.Minor(uint64(stat.Dev)))
	inode := strconv.FormatUint(stat.Ino, 10)
	scanner := bufio.NewScanner(maps)
	var start, end uintptr
	for scanner.Scan() {
		line := scanner.Text()
		if start != 0 {
			value, ok := strings.CutPrefix(line, "Anonymous:")
			if !ok {
				continue
			}
			// A read-only VMA can still contain private relocations.
			if strings.TrimSpace(value) == "0 kB" {
				_, _, errno := unix.Syscall(unix.SYS_MADVISE, start, end-start, unix.MADV_RANDOM)
				if errno == 0 {
					_, _, _ = unix.Syscall(unix.SYS_MADVISE, start, end-start, unix.MADV_DONTNEED)
				}
			}
			start, end = 0, 0
		} else {
			start, end = executablePageRange(line, device, inode)
		}
	}
}

func executablePageRange(line, device, inode string) (uintptr, uintptr) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[3] != device || fields[4] != inode ||
		(fields[1] != "r-xp" && fields[1] != "r--p") {
		return 0, 0
	}
	first, last, _ := strings.Cut(fields[0], "-")
	start, _ := strconv.ParseUint(first, 16, strconv.IntSize)
	end, _ := strconv.ParseUint(last, 16, strconv.IntSize)
	if end <= start {
		return 0, 0
	}
	return uintptr(start), uintptr(end)
}
