# OKRchestra

OKRchestra is a Go-based CLI for OKR-driven agent orchestration. It combines metrics tracking, automated planning, and agent execution to help teams make measurable progress toward their objectives.

## Features

### 🎯 OKR Management
- Define objectives and key results with measurable targets
- Track progress across org, team, and personal scopes
- Automatic status updates based on metrics
- Evidence-based progress tracking

### 📊 Metrics Collection
- **Git metrics**: commits, contributors, lines changed
- **CI metrics**: test coverage, build success rates
- **Manual metrics**: custom metrics via YAML
- Automatic snapshot generation

### 🤖 Agent Orchestration
- Generate AI-driven work plans from OKRs
- Execute plans with pluggable adapters (Codex, etc.)
- Proposal workflow for OKR changes with permissions
- Full audit logging

### 🔔 Automatic Status Updates
- KR status automatically updates when metrics change:
  - `not_started` → `in_progress` (current > baseline)
  - `in_progress` → `achieved` (current >= target)
- macOS notifications for status changes (daemon mode)
- Evidence references added automatically
- Preserves manually-set `blocked` and `at_risk` statuses

### 🔄 Daemon Mode
- Background process for scheduled tasks
- Automatic metric collection
- Plan generation and execution
- Status update notifications

## Quick Start

### Installation

```bash
go install ./cmd/okrchestra
```

### Initialize a Workspace

```bash
okrchestra init --workspace ~/my-project
cd ~/my-project
```

This creates:
- `okrs/` - OKR definitions
- `culture/` - Values and standards
- `metrics/` - Metric inputs and snapshots
- `artifacts/` - Plans and run results
- `audit/` - Audit database

### Measure Progress

```bash
okrchestra kr measure --workspace .
```

Outputs:
```
Status updated: KR-1.1 not_started -> achieved (102/100)
Wrote snapshot: metrics/snapshots/2026-01-18.json
```

### Generate a Plan

```bash
okrchestra plan generate --workspace . --kr-id KR-1.2
```

### Run a Plan

```bash
okrchestra plan run --workspace . --adapter codex artifacts/plans/2026-01-18/plan.json
```

### Start the Daemon

```bash
okrchestra daemon run --workspace .
```

Schedule recurring tasks:
```bash
okrchestra daemon schedule --workspace . \
  --cron "0 9 * * *" \
  --job kr_measure
```

## Workspace Structure

```
my-project/
├── okrs/
│   ├── org.yml           # Organization OKRs
│   ├── permissions.yml   # Agent permissions
│   └── schema.md         # OKR schema reference
├── culture/
│   ├── values.md         # Team values
│   └── standards.md      # Engineering standards
├── metrics/
│   ├── manual.yml        # Manual metrics
│   ├── ci_report.json    # CI/CD metrics
│   └── snapshots/        # Daily metric snapshots
├── artifacts/
│   ├── plans/            # Generated plans
│   ├── runs/             # Plan execution results
│   └── proposals/        # OKR change proposals
└── audit/
    └── audit.sqlite      # Audit log database
```

## OKR Workflow

1. **Define OKRs** in `okrs/org.yml`:
   ```yaml
   objectives:
     - objective_id: OBJ-1
       objective: Ship production-ready feature
       key_results:
         - kr_id: KR-1.1
           description: Complete 100 unit tests
           metric_key: ci.test_count
           baseline: 0
           target: 100
           status: not_started
   ```

2. **Track metrics** in `metrics/manual.yml` or via CI:
   ```yaml
   metrics:
     - key: ci.test_count
       value: 85
       unit: count
   ```

3. **Run measurement**:
   ```bash
   okrchestra kr measure --workspace .
   ```
   Status automatically updates to `in_progress` (85/100)

4. **View score**:
   ```bash
   okrchestra kr score --workspace .
   ```

## Commands

### Workspace
- `init` - Initialize new workspace

### Key Results
- `kr measure` - Collect metrics and update KR status
- `kr score` - Score KRs against targets

### Plans
- `plan generate` - Generate work plan from OKRs
- `plan run` - Execute a plan

### OKRs
- `okr propose` - Propose OKR changes
- `okr apply` - Apply approved proposal

### Daemon
- `daemon run` - Start daemon
- `daemon schedule` - Schedule recurring jobs
- `daemon jobs` - List jobs
- `daemon launchd` - Generate macOS launchd plist

## Configuration

### Metrics

Edit `metrics/manual.yml` to track custom metrics:
```yaml
metrics:
  - key: manual.feature_count
    value: 5
    unit: count
    evidence:
      - features:auth,dashboard,reports
```

### Permissions

Control agent access in `okrs/permissions.yml`:
```yaml
default_policy: deny
permissions:
  - agent: team-backend
    can_propose_for:
      - owner: team-backend
```

## Notifications

When running the daemon on macOS, you'll receive notifications for:
- 🎉 KR achieved
- 🚀 KR in progress
- ✅ Plan completed
- ⚠️ Plan failed

## Culture, OKRs, and Guardrails

- Operational values: `culture/values.md`
- Engineering standards and agent guardrails: `culture/standards.md`
- OKR schema and examples: `okrs/schema.md`, `okrs/org.yml`
- Permissions model: `okrs/permissions.yml`

## Build

```bash
go build ./cmd/okrchestra
```

## Development

```bash
# Run tests
go test ./...

# Install locally
go install ./cmd/okrchestra

# Build for release
go build -o okrchestra ./cmd/okrchestra
```
