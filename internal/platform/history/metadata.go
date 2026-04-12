package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const metaFileName = "meta.json"

// WriteMetadata writes the initial metadata JSON file to the given directory.
// The directory must already exist.
func WriteMetadata(dir string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("history: marshal metadata: %w", err)
	}

	path := filepath.Join(dir, metaFileName)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("history: write metadata: %w", err)
	}

	return nil
}

// ReadMetadata reads and parses the meta.json file from the given directory.
func ReadMetadata(dir string) (Metadata, error) {
	path := filepath.Join(dir, metaFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("history: read metadata: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("history: unmarshal metadata: %w", err)
	}

	return meta, nil
}

// UpdateMetadataExit reads the existing meta.json, sets endedAt and exitCode,
// and writes it back. This is called when a session exits.
func UpdateMetadataExit(dir string, endedAt time.Time, exitCode int) error {
	meta, err := ReadMetadata(dir)
	if err != nil {
		return fmt.Errorf("history: update exit: %w", err)
	}

	meta.EndedAt = &endedAt
	meta.ExitCode = &exitCode

	return WriteMetadata(dir, meta)
}
