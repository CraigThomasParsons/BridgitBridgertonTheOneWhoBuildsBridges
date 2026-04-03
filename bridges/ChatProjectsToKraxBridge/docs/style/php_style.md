# PHP Coding Style and Commenting Conventions (Project Standard)

This document defines the required PHP coding style for Auralis test suites and related tooling.
The core goal is clarity, explicitness, and long-term maintainability.

---

## Core Philosophy

- Code must be readable first and clever second.
- Explicit behavior is preferred over compact tricks.
- Descriptive names are required for variables, functions, and files.
- Future readers should understand intent without external context.
- Runtime paths should be straightforward to trace and debug.

---

## Standards Baseline

This project follows these PHP conventions:

- PSR-12 coding standard baseline.
- Modern PHP semantics with strict type hinting for parameters and return types.
- Explicit visibility (`public`, `protected`, `private`) on all class members.
- Consistent formatting with 4 spaces for indentation.

---

## Naming Conventions (Locked)

### Variables and Properties

- Use `$camelCase`.
- Use descriptive multi-word names (e.g., `$extractedPayload`).
- Never use single-letter variable names except conventional short values (e.g., `$i` in loops).

### Functions and Methods

- Use `camelCase`.
- Function names must be verb-based and explicit.
- Avoid abbreviations and generic names.

---

## Commenting Rules (Critical)

### Comment Density

- **CRITICAL REQUIREMENT:** Add a meaningful comment (1 or more comments) every 3 logical lines or less in all execution paths. This explicitly applies to test code as well.
- Explain intent, expectations, and constraints, not obvious syntax.
- Prioritize *why* over *what*.

### DocBlocks

- Use standard PHPDoc blocks for all classes and methods.

### Inline Comments

- Use line comments strategically to break down blocks of statements and explain assertions.

---

## Final Rule

If a future reader must guess intent, the code is wrong.
Baseline compliance is required.
Clarity is the priority.
