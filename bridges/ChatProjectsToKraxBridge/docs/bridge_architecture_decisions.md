# Bridge Architecture & Decisions

## 1. Project Goal

The `ChatProjectsToKraxBridge` acts as **Layer 1: Meaning Extraction** inside the overarching AI Factory. Its purpose is to take conceptual user conversations and Lean Inception planning data from the `ChatProjects` application, and deterministically transform them into static, Markdown-based Factory Artifacts (`VISION.md`, `PERSONAS.md`, `EPICS.md`, `STORIES.md`, `CONSTRAINTS.md`). These artifacts are then shipped out via the PostalService into Krax's inbox for execution.

## 2. Core Decisions Made

### A. Relying on Hard Lean Inception Data (Eliminating Hallucinations)
**What we did:** We shifted the core bridge architecture to ingest structured Lean Inception data directly from the `ChatProjects` database, rather than trying to use an LLM to "guess" features from raw ChatGPT chat logs. 
**Why:** As discovered in the `ChatGptToChatProjectsBridge` and `PiperStoryArchitect` workflows, the `ChatProjects` app already contains highly structured `Inception`, `InceptionPersona`, and `InceptionFeature` records. By using this strictly defined data as our baseline *Hard Context*, we prevent the extraction LLM from hallucinating new features or dropping scope. The LLM is only leveraged to *expand* these existing features into proper Epics and Stories using the raw conversation logs as *Soft Context*.

### B. Choosing Rust for the Extractor Core
**What we did:** We opted to build the `ChatProjectsToKraxBridge` in **Rust**, explicitly avoiding Python unless absolutely necessary.
**Why:**
- Over time, Python workflows frequently become fragile, weakly typed, and unpredictable when interacting heavily with the filesystem or complex state machines.
- Rust enforces explicit error handling (preventing silent failures with missing files), strict types, and robust memory safety.
- This aligns structurally with the `Tess Snow` orchestrator pattern established in `TheWritersRoom`.

### C. Enforcing Strict Coding Standards
**What we did:** We have adopted the explicit, highly-commented coding conventions defined in the `/docs/style` directory (specifically `rust_style.md`).
**Why:** Following the philosophy of "Code is written for humans first," all bridge operations must prioritize extreme explicitness over cleverness. This mandates fully descriptive variable names, complete absence of single-letter variables, and dense commenting on the *intent* behind logical blocks. If an agent goes rogue or the bridge corrupts data, a human (or an autonomous AI maintainer) must be able to comprehend the exact execution pipeline at a glance.

## 3. The Required Data Payload

The `/api/projects/{project}/krax-input` API route (or adapted `piper-input` route) must provide:
- The `Project` metadata.
- All associated `Conversation` transcripts.
- The `Inception` DB record (vision, business goals, MVP canvas).
- All `InceptionPersona` records.
- All `InceptionFeature` records.
