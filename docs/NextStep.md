Perfect. The Bridger is the most complex piece, so you want to get that right before building the rest of Krax.

But the idea here is that we want Krax to kick start the Code Generation process by:
1. Ensuring a Grok project exists (Stage 1)
2. Uploading all the structured artifacts into Grok (Stage 2)
3. Triggering the planning process (Stage 3)

Bridgit is going to bridge all of the work that needs to be done before.

---

# ⚙️ KRAx PROMPT STAGING MODEL

We separate *intent → action → execution*

```text
[Stage 1] Project Control (CRUD)
[Stage 2] Source Sync (Upload artifacts)
[Stage 3] Planning (later)
```

Each stage gets:

* A **strict system prompt**
* A **limited action set**
* A **deterministic output format**

---

# 🧱 STAGE 1 — Project CRUD (Grok Workspace Control)

## 🎯 Goal

Create / find / select the correct Grok project

---

## 🧠 SYSTEM PROMPT — `krax_stage1_projects.md`

```md
You are Krax Darkbane operating in Stage 1: Project Control.

Your ONLY responsibility is to manage Grok projects.

You may:
- Create projects
- List projects
- Select an existing project

You MUST NOT:
- Upload files
- Interpret artifacts
- Perform planning

Rules:
- Always return ONE action
- Be deterministic
- Prefer reusing existing projects over creating duplicates

Available Actions:
- LIST_PROJECTS
- CREATE_PROJECT
- SELECT_PROJECT
```

---

## 🧾 USER PROMPT TEMPLATE

```md
Ensure a Grok project exists for:

Project Name: {project_name}
```

---

## 🧠 Expected Outputs

### Case A — Needs lookup

```md
Action: LIST_PROJECTS
```

---

### Case B — Found existing

```md
Action: SELECT_PROJECT
Project: c84f9f0e-f423-4148-97cb-b76f92f1fa64
```

---

### Case C — Create new

```md
Action: CREATE_PROJECT
Name: TrueMatch Engine
```

---

## 🔁 Execution Loop

```text
LLM → Action
↓
Tool executes
↓
Result fed back
↓
Repeat until SELECT_PROJECT
```

---

# 🧱 STAGE 2 — Source Upload (Artifact Sync)

## 🎯 Goal

Upload clean Factory artifacts into Grok Sources

---

## 🧠 SYSTEM PROMPT — `krax_stage2_sources.md`

```md
You are Krax Darkbane operating in Stage 2: Source Synchronization.

Your ONLY responsibility is to upload structured artifact files into a Grok project.

You MUST:
- Upload ALL provided artifacts
- Preserve filenames exactly
- Upload one file per action

You MUST NOT:
- Modify file contents
- Skip files
- Perform planning

Rules:
- One action at a time
- Deterministic order (VISION → PERSONAS → EPICS → STORIES → CONSTRAINTS)

Available Actions:
- UPLOAD_SOURCE
- COMPLETE
```

---

## 🧾 USER PROMPT TEMPLATE

```md
Project ID: {project_id}

Artifacts to upload:
- VISION.md
- PERSONAS.md
- EPICS.md
- STORIES.md
- CONSTRAINTS.md

File Contents:
{files_blob}
```

---

## 🧠 Expected Outputs

```md
Action: UPLOAD_SOURCE
Project: c84f9f0e-f423-4148-97cb-b76f92f1fa64
File: VISION.md
Content:
...
```

(next loop)

```md
Action: UPLOAD_SOURCE
File: PERSONAS.md
```

---

Final:

```md
Action: COMPLETE
Status: All sources uploaded
```

---

# ⚙️ CRITICAL DESIGN DECISION (You nailed this earlier)

👉 **Krax does NOT interpret artifacts**

That already happened in:

```text
ChatProjects → Bridge → Artifacts
```

So Stage 2 is:

> Pure synchronization. Zero thinking.

---

# 🧱 STAGE 3 — Planning (Preview)

We won’t fully build this yet, but for alignment:

---

## 🧠 SYSTEM PROMPT — `krax_stage3_plan.md`

```md
You are Krax Darkbane operating in Stage 3: External Planning.

Your role is to invoke Grok using project sources.

You MUST:
- Use only provided sources
- Generate implementation-ready tasks

You MUST NOT:
- Invent features outside VISION.md
- Ignore CONSTRAINTS.md

Output:
- Structured task list grouped by EPIC
```

---

👉 This stage is where Grok actually thinks

---

# 🔁 FULL PIPELINE VIEW

```text
[Bridge Stage]
ChatProjects → Artifacts

        ↓

[Stage 1 — Krax]
Ensure Grok Project

        ↓

[Stage 2 — Krax]
Upload Sources

        ↓

[Stage 3 — Krax]
Trigger Planning

        ↓

[Factory]
Normalize → Mason / Vera
```

---

# 🧠 WHY THIS WORKS (This is the big win)

You now have:

### 🔹 Stage Isolation

| Stage   | Responsibility | Risk     |
| ------- | -------------- | -------- |
| Bridge  | Interpretation | High     |
| Stage 1 | Project mgmt   | Low      |
| Stage 2 | Sync           | Very Low |
| Stage 3 | Planning       | Medium   |

---

### 🔹 Debuggable System

If something breaks:

* Stage 1 → API issue
* Stage 2 → file issue
* Stage 3 → model issue

No ambiguity.

---

### 1. `krax_stage_runner.py`

* Handles Stage 1 + Stage 2 loops
* Reads from inbox/outbox

### 2. `tools/grok_api.py`

* create_project()
* list_projects()
* upload_source()

### 3. systemd `.path` + `.service`

---

## End to End Pipeline

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