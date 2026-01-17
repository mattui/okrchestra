# Watch Triggers

OKRchestra implements a poll-based watch system that monitors files and directories for changes, automatically triggering follow-up jobs when changes are detected.

## Overview

The watch system runs as part of the daemon's scheduler, executing `watch_tick` jobs every 30 seconds to check for file changes.

## Watched Resources

### 1. OKRs Directory (`<workspace>/okrs`)

**What it watches:** All `.yml` and `.yaml` files in the `okrs` directory  
**Trigger:** When human-applied OKR proposals are detected  
**Follow-up jobs:**
- `kr_measure` - Re-measure key results with updated OKRs
- `plan_generate` - Generate new plans based on OKR changes

### 2. Manual Metrics (`<workspace>/metrics/manual.yml`)

**What it watches:** The `manual.yml` file in the metrics directory  
**Trigger:** When manual metric data is updated  
**Follow-up jobs:**
- `kr_measure` - Re-calculate key results with updated metrics

### 3. Plans Directory (`<workspace>/artifacts/plans`)

**What it watches:** All `.json` files (specifically `plan.json`) in plan subdirectories  
**Trigger:** When new plans are generated  
**Follow-up jobs:**
- `plan_execute` - Automatically execute newly generated plans (optional)

## How It Works

1. **Scheduling:** The scheduler automatically enqueues `watch_tick` jobs every 30 seconds
2. **State Tracking:** Uses `daemon_kv` table to store file hashes and modification times
3. **Change Detection:** Compares current file state with stored state using SHA256 hashes
4. **Job Enqueueing:** When changes are detected, appropriate follow-up jobs are enqueued with `EnqueueUnique` to prevent duplicates

## Implementation Details

### File State Tracking

Each watched file/directory maintains state in the `daemon_kv` table:

```json
{
  "path": "/path/to/file.yml",
  "mod_time": "2024-01-01T10:00:00Z",
  "hash": "sha256_hash_of_contents",
  "last_seen": "2024-01-01T10:00:30Z"
}
```

### Change Detection Logic

- **Files:** Detects creation, modification (via hash), and deletion
- **Directories:** Recursively walks directory, tracking all matching files
- **File Types:** Only watches `.yml`, `.yaml`, and `.json` files
- **Hash-based:** Uses content hashing to detect actual changes, not just mtime

### Job Handler

The `handleWatchTick` handler:
1. Checks each watched resource for changes
2. Enqueues appropriate follow-up jobs when changes detected
3. Returns a result indicating changes found and actions taken
4. Uses `EnqueueUnique` to prevent duplicate jobs

### Scheduler Integration

The `scheduleWatchTicks` method in the scheduler:
- Schedules `watch_tick` jobs at 30-second intervals
- Uses watermark-based scheduling to catch up on missed intervals
- Integrates with existing scheduler tick logic

## Configuration

The watch interval is hardcoded to 30 seconds. To modify:

```go
// In internal/daemon/watch.go
func (s *Scheduler) scheduleWatchTicks(lastWatermark, now time.Time) error {
    interval := 30 * time.Second  // Change this value
    // ...
}
```

## Usage Example

When the daemon is running, watch triggers automatically:

1. **User edits OKRs:**
   ```yaml
   # okrs/org.yml
   objectives:
     - id: obj-1
       title: Improve code quality
       key_results:
         - id: kr-1
           metric: test_coverage
           target: 90
   ```

2. **Watch tick detects change** (within 30 seconds)

3. **Jobs are enqueued:**
   - `kr_measure` to recalculate metrics
   - `plan_generate` to create updated plans

4. **Daemon processes jobs** according to normal job scheduling

## Testing

Run watch-specific tests:

```bash
# Unit tests
go test -v ./internal/daemon -run TestWatch

# Integration tests
go test -v ./integration -run TestWatch
```

## Acceptance Criteria

✅ `watch_tick` job type scheduled every 30 seconds  
✅ Monitors `<ws>/okrs` directory for changes  
✅ Monitors `<ws>/metrics/manual.yml` for changes  
✅ Monitors `<ws>/artifacts/plans` for new plans  
✅ Uses `daemon_kv` to store last-seen mtimes/hashes  
✅ Enqueues `kr_measure` when OKRs or manual metrics change  
✅ Enqueues `plan_generate` when OKRs change  
✅ Enqueues `plan_execute` when new plans are generated  
✅ Changes to watched files trigger follow-up jobs

## Future Enhancements

Potential improvements:
- Configurable watch intervals per resource
- Inotify/fsnotify integration for real-time watching (Linux/Mac)
- Selective plan execution based on plan metadata
- Watch for configuration file changes
- Metrics on watch performance and change frequency
