# Guardrails

The `guardrails` package enforces AGENTS.md rules during plan execution to ensure agents operate within defined boundaries.

## Overview

This package provides two primary enforcement mechanisms:

1. **OKR Directory Protection**: Detects and reverts unauthorized modifications to the `okrs/` directory
2. **Result Schema Validation**: Validates `result.json` files against the required schema

## Components

### OKR Directory Protection (`okrs_guard.go`)

Prevents agents from directly editing OKR files, which violates AGENTS.md policy. Agents must propose OKR changes via `result.json` instead.

#### Key Functions

- `NewIntegrityCheck(wsRoot string)`: Creates a new integrity checker that captures the initial state of `okrs/`
- `CaptureAfter()`: Captures the post-execution state
- `HasChanges()`: Returns true if `okrs/` was modified
- `RevertOKRs(wsRoot string)`: Reverts unauthorized changes using `git checkout`
- `WriteViolation(artifactsDir string, violation map[string]any)`: Records violation details

#### Workflow

1. Before adapter execution: Capture hash of `okrs/` directory contents
2. After adapter execution: Capture hash again and compare
3. If changed:
   - Attempt to revert via `git checkout -- okrs`
   - Write `violation.json` to item's artifacts directory
   - Log audit event with type `guardrail_violation`
   - Fail the plan item

### Result Schema Validation (`result_validate.go`)

Enforces strict schema compliance for agent output files per AGENTS.md section 3.

#### Required Schema (version 1.0)

```json
{
  "schema_version": "1.0",
  "summary": "Brief outcome summary",
  "proposed_changes": ["List", "of", "changes"],
  "kr_targets": ["kr-123", "kr-456"],
  "kr_impact_claim": "Quantified impact with evidence"
}
```

#### Rules

- **Required fields**: `schema_version`, `summary`, `proposed_changes`, `kr_targets`, `kr_impact_claim`
- **Schema version**: Must be exactly `"1.0"`
- **No extra fields**: Any fields beyond the required set are rejected
- **Non-empty strings**: `summary` and `kr_impact_claim` cannot be empty
- **Array types**: `proposed_changes` and `kr_targets` must be arrays (can be empty)

#### Key Function

- `ValidateResultJSON(path string) error`: Performs comprehensive validation

### Integration

The guardrails are integrated into `internal/planner/run.go`:

1. **Before adapter run** (line ~119): Create integrity check
2. **After adapter run** (line ~151): Check for OKR changes
3. **Result validation** (line ~208): Validate result.json schema

If either check fails, the plan item is marked as failed and a violation record is created.

## Violation Records

When a guardrail is violated, a `violation.json` file is written to the item's artifacts directory:

```json
{
  "violation_type": "okrs_direct_edit",
  "details": {
    "message": "Agent directly modified okrs/ directory, which is prohibited by AGENTS.md",
    "changed_files": ["okrs/ directory modified (hash mismatch)"],
    "reverted": true,
    "revert_error": "",
    "item_id": "ITEM-1",
    "run_id": "20260117T223000Z"
  }
}
```

## Audit Logging

All guardrail violations are logged to the audit database with:

- **actor**: `"daemon"`
- **event type**: `"guardrail_violation"`
- **payload**: Details about the violation, including what was changed and whether it was reverted

## Testing

Run the guardrails tests:

```bash
go test ./internal/guardrails -v
```

Tests cover:
- Valid and invalid result.json schemas
- Directory hash detection
- Violation record creation
- Workspace root resolution

## Future Enhancements

Possible improvements:

1. **Detailed file-level diff**: Track exactly which files in `okrs/` changed
2. **Configurable policies**: Allow customization of what directories are protected
3. **Evidence validation**: Verify that `kr_impact_claim` references actually exist
4. **Metrics validation**: Ensure referenced metrics exist in `metrics/` directory
