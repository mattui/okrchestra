package daemon

import (
	"fmt"
	"time"
)

// Scheduler manages recurring job scheduling.
type Scheduler struct {
	store    *Store
	location *time.Location
}

// NewScheduler creates a scheduler with the given timezone location.
func NewScheduler(store *Store, tzName string) (*Scheduler, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("load timezone %s: %w", tzName, err)
	}
	return &Scheduler{
		store:    store,
		location: loc,
	}, nil
}

// Tick schedules any jobs that need to be enqueued based on current time.
func (s *Scheduler) Tick(now time.Time) error {
	// Get last watermark
	watermarkStr, err := s.store.GetKV("scheduler_watermark")
	if err != nil {
		return fmt.Errorf("get scheduler watermark: %w", err)
	}

	var lastWatermark time.Time
	if watermarkStr != "" {
		lastWatermark, err = time.Parse(time.RFC3339, watermarkStr)
		if err != nil {
			return fmt.Errorf("parse watermark: %w", err)
		}
	}

	// If this is the first run, set watermark to now and don't schedule past jobs
	if lastWatermark.IsZero() {
		if err := s.store.SetKV("scheduler_watermark", now.UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("set initial watermark: %w", err)
		}
		return nil
	}

	// Schedule kr_measure daily at 02:00 America/Chicago
	if err := s.scheduleDailyAt(lastWatermark, now, "kr_measure", 2, 0); err != nil {
		return fmt.Errorf("schedule kr_measure: %w", err)
	}

	// Schedule plan_generate weekly Monday at 09:00 America/Chicago
	if err := s.scheduleWeeklyAt(lastWatermark, now, "plan_generate", time.Monday, 9, 0); err != nil {
		return fmt.Errorf("schedule plan_generate: %w", err)
	}

	// Schedule plan_execute weekly Monday at 09:15 America/Chicago
	if err := s.scheduleWeeklyAt(lastWatermark, now, "plan_execute", time.Monday, 9, 15); err != nil {
		return fmt.Errorf("schedule plan_execute: %w", err)
	}

	// Schedule watch_tick every 30 seconds
	if err := s.scheduleWatchTicks(lastWatermark, now); err != nil {
		return fmt.Errorf("schedule watch_tick: %w", err)
	}

	// Update watermark
	if err := s.store.SetKV("scheduler_watermark", now.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("update watermark: %w", err)
	}

	return nil
}

// scheduleDailyAt schedules a job daily at the specified hour and minute.
func (s *Scheduler) scheduleDailyAt(lastWatermark, now time.Time, jobType string, hour, minute int) error {
	// Start from the day after lastWatermark
	start := lastWatermark.In(s.location).Truncate(24 * time.Hour).Add(24 * time.Hour)

	for current := start; !current.After(now); current = current.Add(24 * time.Hour) {
		scheduledTime := time.Date(
			current.Year(), current.Month(), current.Day(),
			hour, minute, 0, 0, s.location,
		)

		if scheduledTime.After(lastWatermark) && !scheduledTime.After(now) {
			payload := map[string]any{
				"scheduled_time": scheduledTime.Format(time.RFC3339),
			}
			_, _, err := s.store.EnqueueUnique(jobType, scheduledTime, payload)
			if err != nil {
				return fmt.Errorf("enqueue %s at %s: %w", jobType, scheduledTime, err)
			}
		}
	}

	return nil
}

// scheduleWeeklyAt schedules a job weekly on the specified weekday at hour and minute.
func (s *Scheduler) scheduleWeeklyAt(lastWatermark, now time.Time, jobType string, weekday time.Weekday, hour, minute int) error {
	// Find the first occurrence of the target weekday after lastWatermark
	start := lastWatermark.In(s.location).Truncate(24 * time.Hour)
	
	// Advance to the next target weekday
	for start.Weekday() != weekday {
		start = start.Add(24 * time.Hour)
	}

	for current := start; !current.After(now); current = current.Add(7 * 24 * time.Hour) {
		scheduledTime := time.Date(
			current.Year(), current.Month(), current.Day(),
			hour, minute, 0, 0, s.location,
		)

		if scheduledTime.After(lastWatermark) && !scheduledTime.After(now) {
			payload := map[string]any{
				"scheduled_time": scheduledTime.Format(time.RFC3339),
			}
			_, _, err := s.store.EnqueueUnique(jobType, scheduledTime, payload)
			if err != nil {
				return fmt.Errorf("enqueue %s at %s: %w", jobType, scheduledTime, err)
			}
		}
	}

	return nil
}
