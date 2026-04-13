//go:build linux

package transport

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func platformParentPID(pid int32) (int32, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, fmt.Errorf("read /proc/%d/stat: %w", pid, err)
	}

	content := string(data)
	closeParen := strings.LastIndex(content, ")")
	if closeParen < 0 || closeParen+2 >= len(content) {
		return 0, fmt.Errorf("parse /proc/%d/stat: no closing paren", pid)
	}

	fields := strings.Fields(content[closeParen+2:])
	if len(fields) < 2 {
		return 0, fmt.Errorf("parse /proc/%d/stat: too few fields", pid)
	}

	ppid, err := strconv.ParseInt(fields[1], 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/%d/stat ppid: %w", pid, err)
	}

	return int32(ppid), nil
}
