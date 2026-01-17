package integration_test

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func loadAuditTypes(t *testing.T, dbPath string) map[string]int {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open audit db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	rows, err := db.Query("SELECT type, COUNT(*) FROM events GROUP BY type")
	if err != nil {
		t.Fatalf("query audit events: %v", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	types := make(map[string]int)
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			t.Fatalf("scan audit event: %v", err)
		}
		types[eventType] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit events: %v", err)
	}
	return types
}

func requireAuditEvents(t *testing.T, dbPath string, want []string) {
	t.Helper()
	types := loadAuditTypes(t, dbPath)
	for _, eventType := range want {
		if types[eventType] == 0 {
			t.Fatalf("missing audit event %s in %s", eventType, dbPath)
		}
	}
}
