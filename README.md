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

## First Goal

Implement:

* ChatGptToChatProjectsBridge
* ChatProjectsToKraxBridge
* FilesystemRouterBridge

## Run

```bash
go run ./cmd/bridgit run
```
