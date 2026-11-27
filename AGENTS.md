# AGENTS.md

> [!IMPORTANT]
> **Top-Level Constraint**: Do not modify this file unless explicitly instructed by a human.

## Rules

### 1. English Only

- Use English exclusively across all code, identifiers, comments, logs, error messages, commit messages, and general documentation.
- Non-English languages are permitted only in explicitly designated localized documentation.

### 2. Minimal Comments

- Do not add unnecessary comments; express intent through clear naming and clean code structure.
- Add brief comments only to explain non-obvious rationale or invariants, and strictly avoid lengthy comment blocks.

### 3. Prefer Linters Over Endless Tests

- When encountering bugs, pitfalls, or recurring mistakes, enforce prevention through linters, compiler checks, type constraints, or static analysis rules whenever possible.
- Avoid endlessly accumulating ad-hoc tests for issues that can be prevented statically.
