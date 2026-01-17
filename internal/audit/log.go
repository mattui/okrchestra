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

// Logger writes audit events to a specific SQLite DB path.
type Logger struct {
	DBPath string
}

// NewLogger returns a Logger bound to the provided DB path.
func NewLogger(dbPath string) *Logger {
	return &Logger{DBPath: dbPath}
}

// LogEvent writes an audit event to the SQLite-backed log.
func LogEvent(actor string, eventType string, payload any) error {
	return logEvent("", actor, eventType, payload)
}

// LogEvent writes an audit event to the configured SQLite-backed log.
func (l *Logger) LogEvent(actor string, eventType string, payload any) error {
	if l == nil {
		return logEvent("", actor, eventType, payload)
	}
	return logEvent(l.DBPath, actor, eventType, payload)
}

func logEvent(dbPath string, actor string, eventType string, payload any) error {
	resolved, err := resolveDBPath(dbPath)
	if err != nil {
		return err
	}
	return writeEvent(resolved, actor, eventType, payload)
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

func resolveDBPath(dbPath string) (string, error) {
	if dbPath == "" {
		dbPath = os.Getenv("OKRCHESTRA_AUDIT_DB")
	}
	if dbPath == "" {
		dbPath = defaultAuditPath
	}
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return "", fmt.Errorf("resolve audit db path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", fmt.Errorf("ensure audit db dir: %w", err)
	}
	return absPath, nil
}

func writeEvent(dbPath string, actor string, eventType string, payload any) error {
	db, err := sql.Open("sqlite", dbPath)
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
