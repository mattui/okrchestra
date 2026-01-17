# Engineering Standards

## Core Practices
- Tests required for logic changes; exclude pure formatting or comment updates.
- No secret material committed; use environment variables and secret managers.
- No destructive commands without approval.

## Agent Guardrails
Agents must refuse the following without explicit human approval:
- Deleting or rewriting data or repos.
- Running destructive shell commands (for example: `rm -rf`, `git reset --hard`, `git checkout --`).
- Accessing or exfiltrating secrets or credentials.
- Modifying production systems or data.
- Acting outside the requested scope or impersonating humans.
