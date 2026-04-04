# BridgitTheBridger

> The entity that connects worlds.

## What is this?

Bridgit is a synchronization engine that bridges:

* Conversations → Projects
* Projects → Pipelines
* Pipelines → Agents

It operates on a file-based pipeline system:

```
inbox → process → outbox → archive
```

## Philosophy

* Files over abstractions
* Deterministic pipelines
* Small, composable bridges
* Observe and declare — never mutate the filesystem

## Quick Start

```bash
# Build everything
make all

# Preview mode (safe, no mutations)
make run

# See all available targets
make help
```

## Setup

### GitHub Token (optional but recommended)

A token gives you 5,000 API requests/hr (vs 60 anonymous) and access to private repos.

1. Go to **GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens**
2. Generate new token with:
   - **Repository access**: All repositories
   - **Permissions**: Contents (read) + Metadata (read)
3. Export it:

```bash
export GITHUB_TOKEN="github_pat_your_token_here"
```

4. To persist across sessions:

```bash
echo 'export GITHUB_TOKEN="github_pat_your_token_here"' >> ~/.zshrc
```

### Auto-Adopt Mode

By default, Bridgit runs in preview mode — it reports unregistered repos without modifying the registry.

To enable adoption, edit `internal/config/config.go`:

```go
AutoAdopt: true,
```

Then run again. To start fresh:

```bash
echo 'repo = []' > registry/repo_registry.toml
make run
```

## Sync Phases

| Phase | Name | Description |
|-------|------|-------------|
| 1 | Fetch | Pull data from ChatProjects, GitHub API, and local filesystem |
| 2 | Adopt | Discover unregistered local repos, optionally add to registry |
| 3 | Resolve | Identity matching: git remote → alias → normalized name |
| 4 | Enrich | Cross-link registry entries with GitHub and ChatProjects metadata |
| 5 | Report | Human-readable summary to stdout |

## Makefile Targets

```
make all               Build engine + scaffolder
make build             Build the main engine binary
make build-scaffolder  Build the scaffolder tool
make test-all          Run all Go tests
make run               Run the sync engine
make route             Run the filesystem router once
make tidy              Tidy go.mod for all modules
make clean             Remove build artifacts
```

## Bridges

| Bridge | Purpose |
|--------|---------|
| ChatGptToChatProjectsBridge | Extracts ChatGPT conversations into project packages |
| ChatProjectsToKraxBridge | Sends project data to Krax for analysis |
| FilesystemRouterBridge | Routes packages between bridge inboxes/outboxes |

## Run

```bash
go run ./cmd/bridgit
```
