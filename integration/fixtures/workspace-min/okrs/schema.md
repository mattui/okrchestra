# OKR YAML Schema (minimal)

Required keys:
- scope: org | team | person
- objectives: list

Each objective:
- objective_id
- objective
- key_results

Each key result:
- kr_id
- description
- owner_id
- metric_key
- baseline
- target
- confidence (0.0-1.0)
- status
- evidence (list of strings)
