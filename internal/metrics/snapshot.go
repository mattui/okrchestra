package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SnapshotSchemaVersion = 1

type Snapshot struct {
	SchemaVersion int           `json:"schema_version"`
	AsOf          string        `json:"as_of"`
	Points        []MetricPoint `json:"points"`
}

func WriteSnapshot(path string, snapshot Snapshot) error {
	if path == "" {
		return fmt.Errorf("snapshot path is required")
	}
	if snapshot.AsOf == "" {
		return fmt.Errorf("snapshot as_of is required")
	}
	snapshot.SchemaVersion = SnapshotSchemaVersion
	snapshot.Points = CanonicalizePoints(snapshot.Points)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure snapshot dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp snapshot: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	return nil
}

func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	var snap Snapshot
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if snap.SchemaVersion != SnapshotSchemaVersion {
		return nil, fmt.Errorf("unsupported snapshot schema_version %d", snap.SchemaVersion)
	}
	if snap.AsOf == "" {
		return nil, fmt.Errorf("snapshot missing as_of")
	}
	snap.Points = CanonicalizePoints(snap.Points)
	return &snap, nil
}

func SnapshotPathForDate(dir string, asOf time.Time) string {
	date := asOf.UTC().Format("2006-01-02")
	return filepath.Join(dir, date+".json")
}

func LatestSnapshotPath(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read snapshots dir: %w", err)
	}
	var candidates []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		// YYYY-MM-DD.json compares lexicographically in chronological order.
		candidates = append(candidates, filepath.Join(dir, name))
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no snapshots found in %s", dir)
	}
	sort.Strings(candidates)
	return candidates[len(candidates)-1], nil
}
