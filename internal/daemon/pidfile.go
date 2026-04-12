package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// WritePIDFile serialises info as JSON and writes it to path with mode 0644.
func WritePIDFile(path string, info Info) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("daemon: marshal pid file: %w", err)
	}

	if err = os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("daemon: write pid file: %w", err)
	}

	return nil
}

// ReadPIDFile reads and deserialises the JSON PID file at path.
func ReadPIDFile(path string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{PID: 0, Version: "", StartedAt: time.Time{}}, fmt.Errorf("daemon: read pid file: %w", err)
	}

	var info Info

	if err = json.Unmarshal(data, &info); err != nil {
		return Info{PID: 0, Version: "", StartedAt: time.Time{}}, fmt.Errorf("daemon: unmarshal pid file: %w", err)
	}

	return info, nil
}

// RemovePIDFile deletes the file at path. It returns nil if the file does not exist.
func RemovePIDFile(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("daemon: remove pid file: %w", err)
	}

	return nil
}

// CheckPIDFile reads the PID file at path and checks whether the recorded
// process is still alive. If the file does not exist, running is false and
// err is nil.
func CheckPIDFile(path string) (running bool, info Info, err error) {
	info, err = ReadPIDFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, Info{PID: 0, Version: "", StartedAt: time.Time{}}, nil
		}

		return false, Info{PID: 0, Version: "", StartedAt: time.Time{}}, err
	}

	return processRunning(info.PID), info, nil
}

// processRunning returns true when the process identified by pid is alive,
// determined by sending signal 0.
func processRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = proc.Signal(syscall.Signal(0))

	return err == nil
}
