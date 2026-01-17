package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const defaultAuditPath = "audit/events.db"

// LogEvent writes an audit event to the SQLite-backed log.
func LogEvent(actor string, eventType string, payload any) error {
	dbPath := os.Getenv("OKRCHESTRA_AUDIT_DB")
	if dbPath == "" {
		dbPath = defaultAuditPath
	}
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("resolve audit db path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("ensure audit db dir: %w", err)
	}

	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return fmt.Errorf("open audit db: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := ensureSchema(db); err != nil {
		return err
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	_, err = db.Exec(
		"INSERT INTO events (ts, actor, type, payload_json) VALUES (?, ?, ?, ?)",
		time.Now().UTC(),
		actor,
		eventType,
		string(payloadJSON),
	)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}

	return nil
}

func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts DATETIME NOT NULL,
			actor TEXT NOT NULL,
			type TEXT NOT NULL,
			payload_json TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create audit schema: %w", err)
	}
	return nil
}
