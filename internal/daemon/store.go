package daemon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store manages daemon state in SQLite.
type Store struct {
	DBPath string
	db     *sql.DB
}

// Job represents a queued or running daemon job.
type Job struct {
	ID             string
	Type           string
	Status         string
	ScheduledAt    time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	PayloadJSON    string
	ResultJSON     string
	LeaseOwner     string
	LeaseExpiresAt *time.Time
}

// Run represents a daemon run record.
type Run struct {
	ID          string
	StartedAt   time.Time
	FinishedAt  *time.Time
	Status      string
	SummaryJSON string
}

// Open opens or creates the daemon state database.
func Open(path string) (*Store, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve daemon db path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, fmt.Errorf("ensure daemon db dir: %w", err)
	}

	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return nil, fmt.Errorf("open daemon db: %w", err)
	}

	store := &Store{
		DBPath: absPath,
		db:     db,
	}

	if err := store.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) ensureSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS daemon_runs (
	id TEXT PRIMARY KEY,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	status TEXT NOT NULL,
	summary_json TEXT
);

CREATE TABLE IF NOT EXISTS daemon_jobs (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	status TEXT NOT NULL,
	scheduled_at TEXT NOT NULL,
	started_at TEXT,
	finished_at TEXT,
	payload_json TEXT,
	result_json TEXT,
	lease_owner TEXT,
	lease_expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_scheduled ON daemon_jobs(status, scheduled_at);
CREATE INDEX IF NOT EXISTS idx_jobs_type_scheduled ON daemon_jobs(type, scheduled_at);

CREATE TABLE IF NOT EXISTS daemon_kv (
	key TEXT PRIMARY KEY,
	value TEXT
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("create daemon schema: %w", err)
	}
	return nil
}

// EnqueueUnique enqueues a job if no job with the same type and scheduled_at exists.
// Returns (jobID, created, error). created is true if a new job was inserted.
func (s *Store) EnqueueUnique(jobType string, scheduledAt time.Time, payload any) (string, bool, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", false, fmt.Errorf("marshal payload: %w", err)
	}

	scheduledAtStr := scheduledAt.UTC().Format(time.RFC3339)
	jobID := fmt.Sprintf("%s_%s", jobType, scheduledAt.UTC().Format("2006-01-02T15:04:05"))

	// Check if job already exists with this type and scheduled_at
	var existingID string
	err = s.db.QueryRow(
		"SELECT id FROM daemon_jobs WHERE type = ? AND scheduled_at = ?",
		jobType, scheduledAtStr,
	).Scan(&existingID)

	if err == nil {
		// Job already exists
		return existingID, false, nil
	}
	if err != sql.ErrNoRows {
		return "", false, fmt.Errorf("check existing job: %w", err)
	}

	// Insert new job
	_, err = s.db.Exec(`
		INSERT INTO daemon_jobs (id, type, status, scheduled_at, payload_json)
		VALUES (?, ?, ?, ?, ?)
	`, jobID, jobType, "queued", scheduledAtStr, string(payloadJSON))

	if err != nil {
		return "", false, fmt.Errorf("insert job: %w", err)
	}

	return jobID, true, nil
}

// ClaimNext atomically claims the next queued job that is ready to run.
func (s *Store) ClaimNext(now time.Time, leaseOwner string, leaseFor time.Duration) (*Job, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	nowStr := now.UTC().Format(time.RFC3339)
	leaseExpiresAt := now.Add(leaseFor).UTC().Format(time.RFC3339)

	// Find next queued job that is ready to run
	var jobID string
	err = tx.QueryRow(`
		SELECT id FROM daemon_jobs
		WHERE status = 'queued' AND scheduled_at <= ?
		ORDER BY scheduled_at ASC
		LIMIT 1
	`, nowStr).Scan(&jobID)

	if err == sql.ErrNoRows {
		return nil, nil // No jobs available
	}
	if err != nil {
		return nil, fmt.Errorf("find next job: %w", err)
	}

	// Claim the job
	startedAt := now.UTC().Format(time.RFC3339)
	_, err = tx.Exec(`
		UPDATE daemon_jobs
		SET status = 'running',
		    started_at = ?,
		    lease_owner = ?,
		    lease_expires_at = ?
		WHERE id = ?
	`, startedAt, leaseOwner, leaseExpiresAt, jobID)

	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Return the claimed job
	return s.GetJob(jobID)
}

// GetJob retrieves a job by ID.
func (s *Store) GetJob(jobID string) (*Job, error) {
	var job Job
	var scheduledAt, startedAt, finishedAt, leaseExpiresAt sql.NullString
	var payloadJSON, resultJSON, leaseOwner sql.NullString

	err := s.db.QueryRow(`
		SELECT id, type, status, scheduled_at, started_at, finished_at,
		       payload_json, result_json, lease_owner, lease_expires_at
		FROM daemon_jobs
		WHERE id = ?
	`, jobID).Scan(
		&job.ID, &job.Type, &job.Status, &scheduledAt,
		&startedAt, &finishedAt, &payloadJSON, &resultJSON,
		&leaseOwner, &leaseExpiresAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}

	if scheduledAt.Valid {
		job.ScheduledAt, _ = time.Parse(time.RFC3339, scheduledAt.String)
	}
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		job.StartedAt = &t
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		job.FinishedAt = &t
	}
	if leaseExpiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, leaseExpiresAt.String)
		job.LeaseExpiresAt = &t
	}
	if payloadJSON.Valid {
		job.PayloadJSON = payloadJSON.String
	}
	if resultJSON.Valid {
		job.ResultJSON = resultJSON.String
	}
	if leaseOwner.Valid {
		job.LeaseOwner = leaseOwner.String
	}

	return &job, nil
}

// Succeed marks a job as succeeded.
func (s *Store) Succeed(jobID string, result any) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	finishedAt := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`
		UPDATE daemon_jobs
		SET status = 'succeeded',
		    finished_at = ?,
		    result_json = ?
		WHERE id = ?
	`, finishedAt, string(resultJSON), jobID)

	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	return nil
}

// Fail marks a job as failed.
func (s *Store) Fail(jobID string, jobErr error) error {
	result := map[string]string{
		"error": jobErr.Error(),
	}
	resultJSON, _ := json.Marshal(result)

	finishedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE daemon_jobs
		SET status = 'failed',
		    finished_at = ?,
		    result_json = ?
		WHERE id = ?
	`, finishedAt, string(resultJSON), jobID)

	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	return nil
}

// ListJobs returns up to limit jobs ordered by scheduled_at.
func (s *Store) ListJobs(limit int) ([]Job, error) {
	rows, err := s.db.Query(`
		SELECT id, type, status, scheduled_at, started_at, finished_at,
		       payload_json, result_json, lease_owner, lease_expires_at
		FROM daemon_jobs
		ORDER BY scheduled_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query jobs: %w", err)
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

// ListRunning returns all jobs with status 'running'.
func (s *Store) ListRunning() ([]Job, error) {
	rows, err := s.db.Query(`
		SELECT id, type, status, scheduled_at, started_at, finished_at,
		       payload_json, result_json, lease_owner, lease_expires_at
		FROM daemon_jobs
		WHERE status = 'running'
		ORDER BY scheduled_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query running jobs: %w", err)
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

// ListQueued returns all jobs with status 'queued' ordered by scheduled_at.
func (s *Store) ListQueued(limit int) ([]Job, error) {
	rows, err := s.db.Query(`
		SELECT id, type, status, scheduled_at, started_at, finished_at,
		       payload_json, result_json, lease_owner, lease_expires_at
		FROM daemon_jobs
		WHERE status = 'queued'
		ORDER BY scheduled_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query queued jobs: %w", err)
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

// ListRecentCompleted returns recently completed jobs (succeeded or failed).
func (s *Store) ListRecentCompleted(limit int) ([]Job, error) {
	rows, err := s.db.Query(`
		SELECT id, type, status, scheduled_at, started_at, finished_at,
		       payload_json, result_json, lease_owner, lease_expires_at
		FROM daemon_jobs
		WHERE status IN ('succeeded', 'failed')
		ORDER BY finished_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query completed jobs: %w", err)
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

func (s *Store) scanJobs(rows *sql.Rows) ([]Job, error) {
	var jobs []Job
	for rows.Next() {
		var job Job
		var scheduledAt, startedAt, finishedAt, leaseExpiresAt sql.NullString
		var payloadJSON, resultJSON, leaseOwner sql.NullString

		err := rows.Scan(
			&job.ID, &job.Type, &job.Status, &scheduledAt,
			&startedAt, &finishedAt, &payloadJSON, &resultJSON,
			&leaseOwner, &leaseExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}

		if scheduledAt.Valid {
			job.ScheduledAt, _ = time.Parse(time.RFC3339, scheduledAt.String)
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			job.StartedAt = &t
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			job.FinishedAt = &t
		}
		if leaseExpiresAt.Valid {
			t, _ := time.Parse(time.RFC3339, leaseExpiresAt.String)
			job.LeaseExpiresAt = &t
		}
		if payloadJSON.Valid {
			job.PayloadJSON = payloadJSON.String
		}
		if resultJSON.Valid {
			job.ResultJSON = resultJSON.String
		}
		if leaseOwner.Valid {
			job.LeaseOwner = leaseOwner.String
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}

	return jobs, nil
}

// GetKV retrieves a value from the key-value store.
func (s *Store) GetKV(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM daemon_kv WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get kv: %w", err)
	}
	return value, nil
}

// SetKV sets a value in the key-value store.
func (s *Store) SetKV(key, value string) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO daemon_kv (key, value)
		VALUES (?, ?)
	`, key, value)
	if err != nil {
		return fmt.Errorf("set kv: %w", err)
	}
	return nil
}
