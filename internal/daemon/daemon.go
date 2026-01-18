package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"okrchestra/internal/audit"
	"okrchestra/internal/notify"
	"okrchestra/internal/workspace"
)

// HandlerFunc is the function signature for job handlers.
type HandlerFunc func(ctx context.Context, ws *workspace.Workspace, job *Job) (any, error)

// Daemon is a long-running process that claims and executes jobs.
type Daemon struct {
	Workspace    *workspace.Workspace
	Store        *Store
	Scheduler    *Scheduler
	Handlers     map[string]HandlerFunc
	AuditLogger  *audit.Logger
	Notifier     *notify.Notifier
	LeaseOwner   string
	LeaseFor     time.Duration
	PollInterval time.Duration
}

// Config holds daemon configuration.
type Config struct {
	Workspace      *workspace.Workspace
	StorePath      string
	TimeZone       string
	LeaseOwner     string
	LeaseFor       time.Duration
	PollInterval   time.Duration
	Notifications  bool
}

// New creates a new daemon with default handlers.
func New(cfg Config) (*Daemon, error) {
	store, err := Open(cfg.StorePath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	scheduler, err := NewScheduler(store, cfg.TimeZone)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("create scheduler: %w", err)
	}

	if cfg.LeaseOwner == "" {
		hostname, _ := os.Hostname()
		cfg.LeaseOwner = fmt.Sprintf("daemon-%s-%d", hostname, os.Getpid())
	}

	if cfg.LeaseFor == 0 {
		cfg.LeaseFor = 30 * time.Second
	}

	if cfg.PollInterval == 0 {
		cfg.PollInterval = 1 * time.Second
	}

	d := &Daemon{
		Workspace:    cfg.Workspace,
		Store:        store,
		Scheduler:    scheduler,
		Handlers:     DefaultHandlers(),
		AuditLogger:  audit.NewLogger(cfg.Workspace.AuditDBPath),
		Notifier:     &notify.Notifier{Enabled: cfg.Notifications},
		LeaseOwner:   cfg.LeaseOwner,
		LeaseFor:     cfg.LeaseFor,
		PollInterval: cfg.PollInterval,
	}

	return d, nil
}

// RegisterHandler registers a handler for a specific job type.
func (d *Daemon) RegisterHandler(jobType string, handler HandlerFunc) {
	d.Handlers[jobType] = handler
}

// Run starts the daemon run loop.
func (d *Daemon) Run(ctx context.Context) error {
	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	// Log daemon start
	startPayload := map[string]any{
		"workspace":     d.Workspace.Root,
		"lease_owner":   d.LeaseOwner,
		"lease_for":     d.LeaseFor.String(),
		"poll_interval": d.PollInterval.String(),
	}
	if err := d.AuditLogger.LogEvent("daemon", "daemon_started", startPayload); err != nil {
		fmt.Fprintf(os.Stderr, "audit log failed: %v\n", err)
	}

	// Run loop
	ticker := time.NewTicker(d.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Graceful shutdown
			stopPayload := map[string]any{
				"workspace": d.Workspace.Root,
			}
			_ = d.AuditLogger.LogEvent("daemon", "daemon_stopped", stopPayload)
			return nil

		case <-ticker.C:
			// Tick scheduler before claiming
			if err := d.Scheduler.Tick(time.Now()); err != nil {
				fmt.Fprintf(os.Stderr, "scheduler tick failed: %v\n", err)
			}

			// Try to claim and execute a job
			if err := d.claimAndExecute(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "job execution failed: %v\n", err)
			}
		}
	}
}

func (d *Daemon) claimAndExecute(ctx context.Context) error {
	job, err := d.Store.ClaimNext(time.Now(), d.LeaseOwner, d.LeaseFor)
	if err != nil {
		return fmt.Errorf("claim job: %w", err)
	}

	if job == nil {
		// No jobs available
		return nil
	}

	// Log job start
	startPayload := map[string]any{
		"job_id":   job.ID,
		"job_type": job.Type,
		"payload":  job.PayloadJSON,
	}
	if err := d.AuditLogger.LogEvent("daemon", "job_started", startPayload); err != nil {
		fmt.Fprintf(os.Stderr, "audit log failed: %v\n", err)
	}

	// Execute job
	handler, ok := d.Handlers[job.Type]
	if !ok {
		err := fmt.Errorf("no handler for job type: %s", job.Type)
		_ = d.Store.Fail(job.ID, err)
		
		failPayload := map[string]any{
			"job_id":   job.ID,
			"job_type": job.Type,
			"error":    err.Error(),
		}
		_ = d.AuditLogger.LogEvent("daemon", "job_failed", failPayload)
		return err
	}

	// Add store and notifier to context for handlers that need them
	ctxWithStore := context.WithValue(ctx, "daemon_store", d.Store)
	ctxWithNotifier := context.WithValue(ctxWithStore, "daemon_notifier", d.Notifier)
	result, execErr := handler(ctxWithNotifier, d.Workspace, job)

	if execErr != nil {
		_ = d.Store.Fail(job.ID, execErr)
		
		failPayload := map[string]any{
			"job_id":   job.ID,
			"job_type": job.Type,
			"error":    execErr.Error(),
		}
		_ = d.AuditLogger.LogEvent("daemon", "job_failed", failPayload)
		return execErr
	}

	// Mark success
	if err := d.Store.Succeed(job.ID, result); err != nil {
		return fmt.Errorf("mark job succeeded: %w", err)
	}

	successPayload := map[string]any{
		"job_id":   job.ID,
		"job_type": job.Type,
		"result":   result,
	}
	_ = d.AuditLogger.LogEvent("daemon", "job_succeeded", successPayload)

	return nil
}

// Close closes the daemon's store.
func (d *Daemon) Close() error {
	return d.Store.Close()
}
