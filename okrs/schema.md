# OKR YAML Schema

This schema defines the required structure for OKR YAML files.

## Top-level
Required:
- `scope`: one of `org`, `team`, `person`
- `objectives`: list of objective objects

## Objective
Required:
- `objective_id`: string, unique within scope
- `objective`: string, human-readable objective statement
- `key_results`: list of key result objects

Optional:
- `owner_id`: string
- `notes`: string

## Key Result
Required:
- `kr_id`: string, unique within objective
- `description`: string
- `owner_id`: string
- `metric_key`: string
- `baseline`: number
- `target`: number
- `confidence`: number between 0.0 and 1.0
- `status`: string
- `evidence`: list of strings

Optional:
- `current`: number
- `last_updated`: string (ISO-8601 date)

## Status
Recommended values: `not_started`, `in_progress`, `at_risk`, `achieved`, `blocked`.
