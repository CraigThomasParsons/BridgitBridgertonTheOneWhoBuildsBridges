# Krax ↔ Grok Pipeline Sprints

This document outlines the structured sprints required to give Krax the autonomous capability to manage Grok projects natively, bridging the output of the `ChatProjectsToKraxBridge` directly into the `Grok` execution environment.

## Context & Prerequisites
The Bridge is currently delivering cleanly separated, hallucination-free Markdown files (`VISION.md`, `PERSONAS.md`, `EPICS.md`, etc.) to Krax's physical Inbox. Krax now needs the deterministic tools to push this data to Grok without interpretive loss.

---

## 🏃 Sprint 1: Grok Project CRUD (The Foundation)

**Goal:** Provide Krax with the mechanical API wrappers and internal tool schemas to Create, Read, Update, and Delete isolated Grok environments. Krax should not be "thinking" here; it must purely act as the routing tool executing CRUD instructions.

### Tasklist: Project CRUD
- [ ] **Task 1: API Boundary Analysis**
  - Read Grok/X.ai API documentation confirming endpoints for Project workspaces.
- [ ] **Task 2: Auth & Config Expansion**
  - Add API keys to Krax's internal `.env` and `Tys` configuration specific to Grok Project contexts.
- [ ] **Task 3: Implement `create_grok_project` Tool**
  - Define the JSON schema for this tool.
  - Implement execution logic: Call the API, receive the new `project_id`, and log the relation locally.
- [ ] **Task 4: Implement `get_grok_project` / `list_projects` Tools**
  - Implement logic to fetch existing projects to prevent blind duplicates.
- [ ] **Task 5: Implement `update` / `delete` Tools**
  - Allow Krax to clear or refresh environments during a failed sprint iteration.
- [ ] **Task 6: Unit Verification**
  - Run the standalone tools via Krax's CLI or test harness to manually verify identical state sync with Grok.

---

## 🏃 Sprint 2: Source File Upload Sync (The Payload)

**Goal:** Enable Krax to physically upload the static artifacts (`VISION.md`, `PERSONAS.md`, `EPICS.md`) from its local inbox to the correctly targeted Grok project context.

### Tasklist: File Synchronization
- [ ] **Task 1: Identify Upload API Contract**
  - Determine if Grok takes raw text payloads attached to the project metadata or if there's a distinct file upload/attachment endpoint.
- [ ] **Task 2: Implement `upload_source_artifact` Tool**
  - Define schema: `{"project_id": "...", "artifact_name": "VISION.md", "content": "..."}` or `{"filepath": "..."}`.
  - Write handling logic securely reading from the localized `inbox/{project}/` folders.
- [ ] **Task 3: Automate File Iteration**
  - Ensure Krax can systematically loop through all artifacts dictated in the `letter.toml` manifest without skipping.
- [ ] **Task 4: Overwrite & Conflict Resolution**
  - If `EPICS.md` changes, Krax must successfully invalidate/overwrite the old file in Grok so Grok's execution context remains clean and up to date.
- [ ] **Task 5: End-to-End Artifact Sync Test**
  - Drop a dummy project into the inbox. Ensure Krax creates the Grok project (Sprint 1) and uploads all dummy files deterministically.

---

## 🏃 Sprint 3: Pipeline Orchestration (The Assembly Line)

**Goal:** Once the Project exists and the Source Files are in Grok's context, coordinate the automated sequence telling Grok *what* to do with them (Pipeline execution).

### Tasklist: Orchestration Loop
- [ ] **Task 1: Define the Execution Prompt**
  - Develop the raw instructions to be launched inside the Grok project. Example: "Read `VISION.md` and `EPICS.md` to formulate the overarching Technical Architecture diagram."
- [ ] **Task 2: Implement `dispatch_pipeline` Tool**
  - A tool for Krax to signal Grok to begin analyzing the attached text/files and generating an answer.
- [ ] **Task 3: Event Tracking & Polling**
  - Krax must correctly identify when Grok is finished reasoning and return the specific output payload.
- [ ] **Task 4: Pipeline Output Routing**
  - Define where the resulting architectures/code outputs are dropped (e.g., pushed out via ThePostalService to the `Next Stage`).
- [ ] **Task 5: Full Factory Handoff Simulation**
  - Run the complete loop:
    1. Bridge drops `ChatProjects` data into Krax.
    2. Krax creates Grok Project (Sprint 1).
    3. Krax uploads Artifacts (Sprint 2).
    4. Krax triggers Pipeline execution (Sprint 3).
    5. Resulting structural code is deposited in outbox.
