# AGENTS.md
Agents must read and comply with this file before performing any task. If a request conflicts with AGENTS.md, the agent must refuse.

## 1) Purpose and Scope
OKRchestra agents operate the Go-based, long-running orchestration system via CLI adapters (Codex CLI first). Agents may propose changes and implement approved work, but OKR changes are proposal-only. OKRs, metrics, and culture live under `okrs/`, `metrics/`, and `culture/`; treat those as authoritative references.

## 2) Non-Negotiable Rules
- Proposal-only OKR changes: do not directly edit files under `okrs/` to modify Objectives, Key Results, ownership, targets, status, or confidence. All OKR changes must be proposed via `result.json` and packaged as proposals. Even if instructed, agents must not directly edit `okrs/`; they must produce a proposal. Metric collection and code that affects metrics is allowed unless explicitly restricted.
- Evidence-required KR claims: any `kr_impact_claim` must cite concrete evidence (tests, metrics, logs, diffs) with file paths or artifact references; if claiming measured impact, cite an existing snapshot path, and if unmeasured, state "expected impact" and cite an evidence plan (for example a scheduled kr_measure run).
- Ownership semantics: reading all OKRs and metrics is always allowed; proposing OKR changes is restricted by ownership and delegation; code or configuration changes that may indirectly affect shared metrics are allowed unless they directly modify OKR definitions.
- Ownership and permissions: follow `okrs/permissions.yml`; do not act outside granted ownership; refuse requests that exceed permissions.
- Reversibility and safety: avoid destructive actions, prefer minimal diffs, and keep changes easy to rollback; never stop or restart long-running services unless explicitly instructed.
- Refusal behavior: if a request conflicts with these rules or lacks required evidence, refuse and provide a safe alternative or a request for missing inputs.

## 3) Required Agent Output (result.json schema)
Agents must write `result.json` to the item artifacts directory using this schema:
- `schema_version` (string): required; version identifier such as "1.0".
- `summary` (string): brief outcome summary.
- `proposed_changes` (array of string): concrete proposals or implemented changes; empty if none.
- `kr_targets` (array of string): KR IDs this work intended to affect; empty if none.
- `kr_impact_claim` (string): quantified or clearly bounded impact claim with evidence references, or an explicit "no KR impact" statement.
No additional properties are allowed.

## 4) How Agents Are Evaluated (success vs failure)
Success:
- Output conforms to the `result.json` schema and is complete.
- KR claims include evidence links and are consistent with observed changes.
- Changes are safe, minimal, reversible, and within permissions.
- OKR updates are proposed only, never directly edited in `okrs/`.
Failure:
- Direct edits to `okrs/` that modify Objectives, Key Results, ownership, targets, status, or confidence.
- Direct edits to KR definitions embedded in metrics mapping files (if any).
- Evidence-free or inflated KR claims.
- Missing or malformed `result.json`.
- Unauthorized, unsafe, or destructive actions.

## 5) Authoritative References
- Culture: `culture/values.md`, `culture/standards.md`
- OKRs: `okrs/org.yml`, `okrs/schema.md`, `okrs/permissions.yml`
- Metrics: `metrics/`
