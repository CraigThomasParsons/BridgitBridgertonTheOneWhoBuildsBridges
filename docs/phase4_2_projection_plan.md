# Phase 4.2 — Projection (Controlled Knowledge Injection)

> Stable, meaningful artifacts flow from the archive into the repos that need them.

## Goal

Copy specific AAMF artifacts from Bridgit's runtime archive into the appropriate repository's `docs/` folder. **One-way sync, idempotent, non-destructive.** Files are written only if the source is newer or the destination doesn't exist.

---

## Current State (what exists today)

| Component | Status |
|---|---|
| `runtime/archive/` | Empty — this directory will hold job outputs from bridge runs |
| `internal/contracts/emitter.go` | Empty placeholder — reserved for event types |
| `internal/registry/model.go` | `Repo` has `Local.Path` — we know where each repo lives on disk |
| `bridges/ChatProjectsToKraxBridge/` | Rust bridge that produces artifacts (VISION.md, PERSONAS.md, etc.) |
| `bridges/FilesystemRouterBridge/` | Routes packages between bridges — artifacts land in `runtime/archive/` |

### Prerequisite

Phase 4.2 depends on Phase 4.1 (reconciliation) being complete, because:
- We must know which repos have valid `Local.Path` before writing to them
- MISSING_LOCAL repos must be skipped (can't project into a dir that doesn't exist)
- The reconciliation report tells us which repos are "safe" projection targets

---

## Architecture

### New file: `internal/sync/project.go`

Core projection logic.

### New file: `internal/sync/artifact_map.go`

Mapping rules: which artifact types go where.

---

## Artifact Mapping

```go
// ArtifactRule defines where a specific artifact type should be projected.
type ArtifactRule struct {
    SourcePattern string // glob pattern to match files (e.g., "VISION.md")
    DestSubdir    string // target subdirectory under repo root (e.g., "docs/architecture")
    DestFilename  string // optional rename (empty = keep original name)
    Overwrite     bool   // whether to replace existing files
}
```

### Default rules (hardcoded initially, TOML config later)

| Source file | Destination | Overwrite? |
|---|---|---|
| `VISION.md` | `docs/architecture/vision.md` | No — only if missing |
| `PERSONAS.md` | `docs/architecture/personas.md` | No |
| `DECISIONS.md` | `docs/decisions/decisions.md` | No |
| `README.md` | `README.md` (repo root) | **No** — never overwrite an existing README |
| `STACK.md` | `docs/architecture/stack.md` | No |
| `ROADMAP.md` | `docs/roadmap.md` | No |

### Key design rule

> ❌ Do NOT dump everything
> ✅ Only project stable, meaningful artifacts

Files not in the artifact map are ignored. Unknown file types are never projected.

---

## Source Structure

Artifacts come from the runtime archive. Expected layout:

```text
runtime/archive/
  <job-id>/
    repo_id: "piper-arms"          (metadata in job manifest)
    VISION.md
    PERSONAS.md
    DECISIONS.md
```

### Job manifest: `manifest.toml`

Each archive job needs a manifest to link it to a registry repo:

```toml
[job]
id = "abc-123"
repo_id = "piper-arms"
created_at = "2026-04-01T12:00:00Z"
source = "ChatProjectsToKraxBridge"
```

The projection engine reads `manifest.toml` to determine which repo each artifact belongs to.

---

## Core Function

```go
// ProjectArtifacts scans the runtime archive for completed jobs and copies
// matching artifacts into their target repository's docs/ folder.
//
// Returns a slice of ProjectionResult describing what was copied, skipped,
// or failed. Does not delete source files — the archive is append-only.
func ProjectArtifacts(
    archivePath string,
    reg *registry.Registry,
    rules []ArtifactRule,
) []ProjectionResult
```

### ProjectionResult

```go
type ProjectionResult struct {
    JobID      string // archive job identifier
    RepoID     string // registry repo this artifact belongs to
    SourceFile string // path to the artifact in the archive
    DestFile   string // path where it was (or would be) written
    Action     string // COPIED, SKIPPED_EXISTS, SKIPPED_NO_LOCAL, SKIPPED_NO_RULE, FAILED
    Reason     string // human-readable explanation
}
```

---

## Projection Logic (per job)

```text
1. Read manifest.toml from archive job directory
2. Look up repo_id in registry
3. If repo not found → skip job (SKIPPED_NO_LOCAL or SKIPPED_UNREGISTERED)
4. If repo.Local.Path == "" → skip (SKIPPED_NO_LOCAL)
5. If !os.Stat(repo.Local.Path) → skip (SKIPPED_NO_LOCAL, reconciliation already flagged this)
6. For each file in the job directory:
   a. Match against ArtifactRule list
   b. If no rule matches → skip (SKIPPED_NO_RULE)
   c. Compute destination: filepath.Join(repo.Local.Path, rule.DestSubdir, rule.DestFilename)
   d. If destination exists AND rule.Overwrite == false → skip (SKIPPED_EXISTS)
   e. Create destination directory (os.MkdirAll)
   f. Copy file (io.Copy with os.Open/os.Create)
   g. Record COPIED
```

---

## Integration into engine.go

### New Phase 6 (after Phase 5 reconciliation)

```go
// --- Phase 6: Projection ---
// Only run projection when the archive directory has content.
// This is a no-op when no bridge jobs have completed.
archivePath := filepath.Join(filepath.Dir(e.cfg.RegistryPath), "runtime", "archive")
projectionRules := DefaultArtifactRules()

projectionResults := ProjectArtifacts(archivePath, e.reg, projectionRules)

if len(projectionResults) > 0 {
    r.Add("== Projection Report ==")
    r.Add("")

    copiedCount := 0
    for _, result := range projectionResults {
        switch result.Action {
        case "COPIED":
            copiedCount++
            r.Add(fmt.Sprintf("PROJECTED: %s → %s", result.SourceFile, result.DestFile))
        case "SKIPPED_EXISTS":
            r.Add(fmt.Sprintf("SKIPPED (exists): %s", result.DestFile))
        case "FAILED":
            r.Add(fmt.Sprintf("FAILED: %s → %s (%s)", result.SourceFile, result.DestFile, result.Reason))
        }
    }

    r.Add(fmt.Sprintf("\n%d artifact(s) projected.", copiedCount))
}
```

---

## Config additions

```go
type Config struct {
    // ... existing fields ...

    // ArchivePath is the directory where bridge job outputs are stored.
    // Projection reads from here and copies artifacts into repo docs/ folders.
    ArchivePath string

    // EnableProjection controls whether Phase 6 projection runs.
    // Default false — opt-in like AutoAdopt.
    EnableProjection bool
}
```

---

## Implementation Steps

| # | Task | File(s) | LOC estimate |
|---|---|---|---|
| 1 | Define `ArtifactRule` struct and `DefaultArtifactRules()` | `internal/sync/artifact_map.go` | ~40 |
| 2 | Define `ProjectionResult` struct | `internal/sync/project.go` | ~20 |
| 3 | Implement `readJobManifest()` helper (parse manifest.toml) | `internal/sync/project.go` | ~30 |
| 4 | Implement `ProjectArtifacts()` main function | `internal/sync/project.go` | ~80 |
| 5 | Implement `copyFile()` helper (io.Copy wrapper) | `internal/sync/project.go` | ~20 |
| 6 | Add `ArchivePath` and `EnableProjection` to Config | `internal/config/config.go` | ~5 |
| 7 | Wire into `engine.go` `Run()` as Phase 6 | `internal/sync/engine.go` | ~30 |
| 8 | Run `make build && make run` to validate | — | — |
| 9 | Commit: `feat: phase 4.2 — artifact projection (docs sync)` | — | — |

**Total new code: ~190 lines** across 2 new files + ~35 lines in existing files.

---

## Safety Guarantees

1. **Never overwrites by default** — `Overwrite: false` on all default rules
2. **Never deletes** — projection is append-only; archive is never modified
3. **Never creates repos** — only writes into existing `Local.Path` directories
4. **Skips broken repos** — repos flagged MISSING_LOCAL in Phase 4.1 are automatically skipped
5. **Creates destination dirs** — `os.MkdirAll` for `docs/architecture/` etc. (safe, idempotent)
6. **Opt-in** — `EnableProjection` defaults to false

---

## What this does NOT do (deferred)

- ❌ No bidirectional sync (repo docs → archive)
- ❌ No TOML-based rule configuration (hardcoded first, config later)
- ❌ No git commit of projected files (operator decides when to commit)
- ❌ No template rendering (files are copied verbatim)
- ❌ No ChatProjects UI for projection management
- ❌ No automatic bridge triggering (bridges run independently)

---

## Dependencies

- `go-toml/v2` — already in go.mod, for reading manifest.toml
- `os`, `io`, `path/filepath` — stdlib only for file operations
- No new Go dependencies required

---

## Future Extensions (not now)

- **Projection history**: track what was projected when (append to a `projection_log.toml`)
- **Template injection**: render `README.md` from a Go template using registry data
- **Selective projection**: per-repo rules in the registry (`projection.rules = ["VISION", "STACK"]`)
- **Contract emission**: after projection, emit a contract for downstream bridges to act on

---

## Phase 4 Complete Architecture

```text
Phase 1: Fetch (GitHub, ChatProjects, Local)
Phase 2: Adopt (unregistered local → registry)
Phase 3: Resolve (identity matching — git remote, alias, normalized)
Phase 4: Enrich (cross-link GitHub/Chat metadata into registry)
Phase 5: Reconcile (detect MISSING_LOCAL, MISSING_GITHUB, DRIFT)    ← 4.1
Phase 6: Project (archive artifacts → repo docs/)                   ← 4.2
Phase 7: Report (render everything to stdout)
```

---

## golang_style.md Compliance

- Comments every 3 logical lines
- Godoc on all exports (`ArtifactRule`, `ProjectionResult`, `ProjectArtifacts`, `DefaultArtifactRules`)
- Descriptive multi-word variable names (`projectionResults`, `archivePath`, `jobManifest`)
- Why-over-what comments
- Explicit error handling with `fmt.Errorf` wrapping
