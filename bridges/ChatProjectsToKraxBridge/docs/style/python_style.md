# Python Coding Style and Commenting Conventions (Project Standard)

This document defines the required Python coding style for the Auralis project and its tooling.
The primary goal is clarity, explicitness, and long-term maintainability.

---

## Core Philosophy

- Code must be readable first and clever second.
- Explicit behavior is preferred over compact tricks.
- Descriptive names are required for variables, functions, and files.
- Future readers should understand intent without external context.
- Runtime paths should be boring, predictable, and easy to debug.

---

## Standards Baseline

This project follows these Python conventions:

- Modern Python 3 semantics (Type hinting where practical).
- Strict enforcement of explicit variable scopes and intent.
- Consistent formatting with 4 spaces for indentation (PEP 8 baseline).

Where standard PEP 8 is silent or flexible, this document defines project-specific rules.

---

## Naming Conventions (Locked)

### Variables and Properties

- Use `snake_case`.
- Use descriptive multi-word names.
- Never use single-letter variable names except conventional short values (e.g., `i` in simple loops).

Good:

```python
decoded_request_payload = json.loads(raw_request_body)
blocker_registry_path = resolve_blocker_registry_path(factory_root)
```

### Constants

- Use `UPPER_SNAKE_CASE` for true constants and module-level immutable values.

Good:

```python
EVENT_STREAM_FILE_NAME = "events.jsonl"
```

### Functions

- Use `snake_case`.
- Function names must be verb-based and explicit.
- Avoid abbreviations and generic names.

Good:

```python
def enqueue_heightmap_job(): pass
def validate_request_payload(): pass
def emit_scrum_blocker_clear_event(): pass
```

---

## File Structure Rules

- One responsibility per file.
- Avoid mixing transport, domain logic, and formatting in the same function.
- Keep I/O boundaries explicit.
- Keep side effects near entry points.

---

## Commenting Rules (Critical)

### Comment Density

- **CRITICAL REQUIREMENT:** Add a meaningful comment (1 or more comments) every 3 logical lines or less in all execution paths.
- Explain intent and constraints, not obvious syntax.
- Prioritize *why* over *what*.

### Docstrings

- Use docstrings for all functions, classes, and modules to document purpose and constraints:

```python
def extract_fenced_code_blocks(text: str) -> list:
    """
    Extracts triple-backtick fenced code blocks from markdown text.
    
    This function explicitly searches for block-level fences and avoids
    matching inline code by requiring them to start on a new line.
    """
    pass
```

### Inline Comments

- Comments should clarify complex logic, linking related steps together.

```python
# Reuse the existing blocker ID so that raise/clear linkage remains stable across retries.
blocker_id = existing_blocker.get('id') or derive_blocker_id(stage, job_id, reason)

# Reject empty payloads early to avoid writing invalid event records down the line.
if not raw_request_body or not raw_request_body.strip():
    raise ValueError("Empty request body")
```

---

## Error Handling Rules

- Fail fast and explicitly.
- Never swallow errors silently or use bare `except:` clauses without re-raising or logging appropriately.
- Use guard clauses before deep nesting.
- Include enough context in errors for operator triage.

Good:

```python
if not os.path.exists(source_path):
    raise FileNotFoundError(f"Missing source file for pipeline: {source_path}")
```

---

## Final Rule

If a future reader must guess intent, the code is wrong.
Baseline compliance is required.
Clarity is the priority.
