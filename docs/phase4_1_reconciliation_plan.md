# Phase 4.1 — Reconciliation (State Alignment)

> Bridgit does not fix the world. She declares where it is broken.

## Goal

Detect misalignment between the three source-of-truth systems (Registry, GitHub, Local filesystem) and produce a deterministic, human-readable reconciliation report. **Read-only — zero mutations.**

---

## Current State (what exists today)

| File | What it does now |
|---|---|
| `internal/sync/engine.go` | 5-phase `Run()` — Fetch → Adopt → Resolve → Enrich → Report. The current "Phase 4" is **enrichment** (cross-linking GitHub/Chat metadata into registry). There is no drift detection. |
| `internal/report/report.go` | Simple `Add(line)` + `Render()` — flat text, no structured status codes. |
| `internal/registry/model.go` | `Repo` struct has `GitHub.Name`, `GitHub.URL`, `Local.Path`, `Chat.ProjectID`. All the data we need to check alignment. |
| `internal/sync/github.go` | `FetchGitHubRepos()` returns `[]GitHubRepo{Name, URL}`. |
| `internal/sync/local.go` | `ScanLocal()` returns `[]LocalRepo{Name, Path}`. |
| `internal/contracts/emitter.go` | Empty placeholder package — reserved for future events. |

---

## Architecture

### New file: `internal/sync/reconcile.go`

Single-purpose file containing all reconciliation logic.

### New type: `ReconcileStatus`

```go
type ReconcileStatus struct {
    RepoID   string   // registry ID (or GitHub name if not registered)
    Status   string   // OK, MISSING_LOCAL, MISSING_GITHUB, UNREGISTERED_GITHUB, DRIFT
    Details  []string // human-readable detail lines
}
```

### New function: `Reconcile()`

```go
func Reconcile(
    reg *registry.Registry,
    githubRepos []GitHubRepo,
    localRepos []LocalRepo,
) []ReconcileStatus
```

Pure function. Takes the three data sets, returns a slice of statuses. No side effects.

---

## Reconciliation Rules

### Check 1 — Registry → Local (MISSING_LOCAL)

For each registry repo where `Local.Path != ""`:
- `os.Stat(repo.Local.Path)` — does the directory exist?
- If not → `MISSING_LOCAL`

```text
MISSING_LOCAL: thedevbacklog
  → expected at /home/craigpar/Code/TheDevBacklog
  → directory does not exist
```

### Check 2 — Registry → GitHub (MISSING_GITHUB)

For each registry repo where `GitHub.Name != ""`:
- Is `GitHub.Name` present in the `githubRepos` slice?
- If not → `MISSING_GITHUB`

```text
MISSING_GITHUB: hiveplan
  → expected GitHub repo: hiveplan
  → not found in GitHub API response (41 repos fetched)
```

### Check 3 — GitHub → Registry (UNREGISTERED_GITHUB)

For each GitHub repo:
- Does any registry entry have `GitHub.Name == ghRepo.Name`?
- Does any registry entry have `ID == ghRepo.Name` (case-insensitive)?
- Does any alias match?
- If none → `UNREGISTERED_GITHUB`

```text
UNREGISTERED_GITHUB: some-new-repo
  → exists on GitHub but not in registry
```

**Note:** This is different from Phase 2's "unregistered local" — Phase 2 handles local dirs not in registry. This handles GitHub repos not in registry.

### Check 4 — Cross-link Drift (DRIFT)

For each registry repo that has BOTH `Local.Path` and `GitHub.Name`:
- Shell out to `git remote get-url origin` on `Local.Path`
- Parse the GitHub owner/repo from the remote URL
- Compare against `GitHub.Name` in registry
- If mismatch → `DRIFT`

```text
DRIFT: piper-arms
  → registry says GitHub name: piper-arms
  → local git remote says: PiperArmsActual
```

### Check 5 — OK

If a registry repo passes all checks → `OK`

---

## Integration into engine.go

### New Phase 5 (renumber current Phase 4 enrichment → Phase 4, reconciliation → Phase 5)

Add after the current enrichment block:

```go
// --- Phase 5: Reconciliation ---
reconcileResults := Reconcile(e.reg, githubRepos, localRepos)

r.Add("== Reconciliation Report ==")
r.Add("")

okCount := 0
for _, result := range reconcileResults {
    switch result.Status {
    case "OK":
        okCount++
    default:
        r.Add(fmt.Sprintf("%s: %s", result.Status, result.RepoID))
        for _, detail := range result.Details {
            r.Add(fmt.Sprintf("  → %s", detail))
        }
    }
}

r.Add(fmt.Sprintf("\n%d OK, %d issues detected", okCount, len(reconcileResults)-okCount))
```

### Report output (example)

```text
== Bridgit Reconciliation Report ==

MISSING_LOCAL: thedevbacklog
  → expected at /home/craigpar/Code/TheDevBacklog
  → directory does not exist

MISSING_GITHUB: hiveplan
  → expected GitHub repo: hiveplan
  → not found in GitHub API response

UNREGISTERED_GITHUB: secret-experiment
  → exists on GitHub but not in registry

DRIFT: piper-arms
  → registry GitHub name: piper-arms
  → local git remote points to: PiperArmsActual

72 OK, 4 issues detected
```

---

## Implementation Steps

| # | Task | File(s) | LOC estimate |
|---|---|---|---|
| 1 | Create `ReconcileStatus` struct | `internal/sync/reconcile.go` | ~15 |
| 2 | Implement `Reconcile()` with checks 1-5 | `internal/sync/reconcile.go` | ~80 |
| 3 | Helper: `githubRepoExists(name, repos)` | `internal/sync/reconcile.go` | ~10 |
| 4 | Helper: `registryHasGitHub(name, reg)` | `internal/sync/reconcile.go` | ~15 |
| 5 | Wire into `engine.go` `Run()` as Phase 5 | `internal/sync/engine.go` | ~25 |
| 6 | Run `make build && make run` to validate | — | — |
| 7 | Commit: `feat: phase 4.1 — reconciliation report (read-only drift detection)` | — | — |

**Total new code: ~145 lines** in one new file + ~25 lines added to engine.go.

---

## What this does NOT do (deferred)

- ❌ No mutations (no cloning, no repo creation, no registry writes)
- ❌ No contract emission (that's Phase 4.2+)
- ❌ No auto-fix suggestions
- ❌ No ChatProjects drift detection (only GitHub and Local for now)
- ❌ No UI integration (ChatProjects registry editor doesn't need changes)

---

## Dependencies

- `internal/git` — reuse `DetectGitHubRemote()` for drift detection (Check 4)
- `os.Stat()` — for local path existence (Check 1)
- No new Go dependencies required

---

## Testing Strategy

Run `make run` and manually verify:
1. Registry repos with valid local paths → OK
2. Registry repos with stale/wrong local paths → MISSING_LOCAL
3. Registry repos with GitHub names not actually on GitHub → MISSING_GITHUB
4. GitHub repos not in registry → UNREGISTERED_GITHUB
5. Repos where git remote disagrees with registry → DRIFT

Future: add `internal/sync/reconcile_test.go` with table-driven tests using mock data.

---

## golang_style.md Compliance

- Comments every 3 logical lines
- Godoc on all exports (`ReconcileStatus`, `Reconcile`)
- Descriptive multi-word variable names (`reconcileResults`, `okCount`, `registryRepo`)
- Why-over-what comments
- Explicit error handling with `fmt.Errorf` wrapping
