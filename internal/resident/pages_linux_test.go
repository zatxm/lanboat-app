package resident

import "testing"

func TestExecutablePageRange(t *testing.T) {
	for _, tc := range []struct {
		mapping string
		start   uintptr
		end     uintptr
	}{
		{"00400000-00410000 r-xp 00000000 103:02 44 /a b", 0x400000, 0x410000},
		{"00410000-00420000 r--p 00010000 103:02 44 /a b", 0x410000, 0x420000},
		{"00420000-00430000 rw-p 00020000 103:02 44 /a b", 0, 0},
		{"00420000-00430000 r--p 00020000 103:03 44 /a b", 0, 0},
		{"00420000-00430000 r--p 00020000 103:02 45 /a b", 0, 0},
	} {
		start, end := executablePageRange(tc.mapping, "103:02", "44")
		if start != tc.start || end != tc.end {
			t.Errorf("%s: got %#x-%#x, want %#x-%#x", tc.mapping, start, end, tc.start, tc.end)
		}
	}
}
