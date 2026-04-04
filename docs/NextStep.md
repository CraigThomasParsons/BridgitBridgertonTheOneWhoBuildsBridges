# Bridgit Sync Engine — Current State & Next Steps

## Completed Phases

| Phase | Name | Status |
|-------|------|--------|
| 1 | Fetch (GitHub + Local scan) | Done |
| 2 | Adopt (auto-register candidates) | Done |
| 3 | Resolve (identity matching via remotes, aliases, normalized names) | Done |
| 4.1 | Reconcile (drift detection: MISSING_LOCAL, MISSING_GITHUB, DRIFT, OK) | Done |
| 4.2 | Project (one-way artifact sync from archive → repo docs/) | Done |
| 4b | Enrich (ChatProjects → registry linking via strict match) | Done |
| 4c | LLM Fuzzy Match (Groq fallback for unresolved ChatProjects names) | Done |
| 5 | Contract/Event system + Intake pipeline | Done |

## Current Architecture

```
Bridge C outbox (AAMF artifacts + letter.toml)
    │
    ├─ [FilesystemRouterBridge Route 2] ──→ runtime/inbox/
    │                                           OR
    └─ [ThePostalService] ─────────────→ runtime/inbox/
                                              │
                                    ┌─────────┘
                                    │ (Engine Phase 5.5: Intake)
                                    │  • parse letter.toml
                                    │  • registry lookup: project_id → repo_id
                                    │  • write manifest.toml
                                    │  • mv → runtime/archive/
                                    ▼
                              runtime/archive/
                                    │
                                    │ (Engine Phase 6: Projection)
                                    │  • read manifest.toml → repo_id
                                    │  • match artifacts to rules
                                    │  • copy to target repo docs/
                                    ▼
                            Target repo docs/ folders
```

## LLM Fuzzy Matching (Phase 4c)

Uses OpenAI-compatible chat completions API (default: Groq llama-3.3-70b-versatile).
Resolves human-readable ChatProjects names to slug-style registry IDs when strict
matching fails (e.g., "The Postal Service" → "thepostalservice").

Config (env vars):
- `LLM_API_KEY` (falls back to `GROQ_API_KEY`)
- `LLM_BASE_URL` (default: `https://api.groq.com/openai/v1`)
- `LLM_MODEL` (default: `llama-3.3-70b-versatile`)

---

## Next Phase: Automated Repo Provisioning

The reconciliation phase already detects MISSING_LOCAL, MISSING_GITHUB, and
UNREGISTERED_GITHUB. The next step is to act on those findings automatically.

### Requirements

1. **No Code folder exists** → Create the local directory under `~/Code/`
2. **No GitHub repo exists** → Create it via `gh repo create`
3. **Code folder exists but no git repo:**
   - If the GitHub repo is empty/new → `git init` + push local code up
   - If there's significant code locally AND it doesn't match GitHub → Flag for manual review
4. **Registry updated** after each provisioning action

### Decision Matrix

| Local State | GitHub State | Action |
|-------------|-------------|--------|
| No folder | No repo | Create both, git init, push |
| No folder | Repo exists | `git clone` into ~/Code/ |
| Folder, no git | Empty repo | `git init`, add remote, push |
| Folder, no git | Repo has code | FLAG FOR REVIEW |
| Folder, git, no remote | Empty repo | Add remote, push |
| Folder, git, no remote | Repo has code | FLAG FOR REVIEW |

### Integration Points

- `gh` CLI (authenticated as CraigThomasParsons)
- `internal/git/git.go` — `DetectGitHubRemote()`
- `internal/sync/reconcile.go` — existing drift detection
- Report system — flag review-needed items clearly

---

## Future: Krax Prompt Staging

Krax uses Bridgit's projected artifacts for LLM-driven code generation.

```
[Bridge Stage] ChatProjects → Artifacts
        ↓
[Stage 1 — Krax] Ensure Grok Project
        ↓
[Stage 2 — Krax] Upload Sources
        ↓
[Stage 3 — Krax] Trigger Planning
        ↓
[Factory] Normalize → Mason / Vera
```

Stage isolation ensures debuggability:
- Stage 1 → API issue
- Stage 2 → file issue
- Stage 3 → model issue
