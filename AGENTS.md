# ZoriDesk Platform Development Guide

Modified by ZoriDesk maintainers on 2026-10-13 to define fork-specific maintenance boundaries. The inherited safety invariants are retained.

## Purpose

ZoriDesk is a local-first platform for composing and governing Agent teams, derived from Loom. Coding is the first reference workflow, not the product boundary.

Natural language may propose an Agent or Team. Only versioned contracts, policy, grants, evidence, and authoritative state transitions may permit execution.

## Repository identity and current state

The maintenance target is ggggggtm/ZoriDesk; the default branch is main. Upstream tttboy123/loom is reference material, not an authorized write target.

Read [STATUS.md](STATUS.md) for current ZoriDesk facts and [TASKS.md](TASKS.md) for the active queue. Inherited docs/CURRENT.md, PROGRESS.md, CHANGELOG.md, .loom-drafts/ and .loom-evidence/ describe upstream history unless explicitly revalidated and labeled otherwise. Their workspace paths, accepted slices and release claims do not establish this fork's current identity or verification.

Before local writes, resolve the physical working directory, Git top-level, remote, branch and existing changes. Compare with the selected ZoriDesk task. Before web or connector writes, verify the repository URL, authenticated identity and write permission for that channel. A logged-in browser does not authenticate a connector or CLI. Stop on mismatch; do not create a substitute repository or use an unrelated checkout.

## Load only what the task needs

Codex already receives the applicable AGENTS.md chain. Do not reopen it merely to start a task. Before modifying a subtree, read its closest scoped AGENTS.md if not already supplied, then only the relevant contract, ADR, architecture section and tests.

Do not preload all plans, logs or archives. Current code and live verification outrank prose. Accepted ADRs explain durable design; TECH-PLAN.md describes the inherited Phase 1 contract, not new ZoriDesk progress. Root repository identity and the user's authorization boundary take precedence over stale upstream destinations in scoped guides.

## Invariants

1. Ordinary conversation never creates a Team. Agent execution requires an explicit Agent trigger.
2. Model-generated or expanded Team drafts require user confirmation before instances or Runs are created.
3. Agent, Harness, Sidecar, and model output is a Proposal or Candidate, never execution authority.
4. Side effects require the applicable policy, contract, grant, and evidence lifecycle.
5. Event Journal facts are append-only. Read models are rebuildable projections, not a second state authority.
6. An executor may submit ready_for_review; it cannot mark its own WorkItem done.
7. Secrets never enter source control, prompts, logs, evidence, or Agent definitions.
8. Root policy, validators, credential boundaries, and authoritative state writers cannot be modified by online self-evolution.

## Development behavior

- Preserve unrelated and pre-existing worktree changes; stop when they conflict with the requested work.
- Recheck repository identity after compaction and before freezing a WorkItem. A thread title, sibling checkout or stale GoalSpec cannot override the actual repository and user-selected scope.
- Continue accepted ZoriDesk work from its recorded branch/commit; do not recreate inherited accepted slices or reuse upstream WorkItem IDs as new work.
- Use one writer per branch or Candidate lineage. Check existing branches and pending review before starting another change.
- Add or update tests before behavior changes. Keep each change within one acceptance boundary and inspect current code before treating a roadmap item as a confirmed defect.
- Use commands supported by the current repository. Do not claim builds, tests, runtime behavior or demos passed without actual evidence. Report skipped checks and unavailable dependencies explicitly.
- Use CURRENT, PARTIAL, TARGET and EXPERIMENTAL precisely. Existing Loopback execution is not a security boundary for arbitrary untrusted code.
- Preserve Apache-2.0, necessary copyright/attribution and upstream history. Distinguish inherited code from independently verified changes; do not backdate, fabricate authorship, create empty activity commits or rewrite history to imply original development.
- Only public upstream and this project's material may be used in public contributions. Never copy employer/internal code, documents, credentials or private work data.
- Do not install dependencies, call paid models, deploy, publish releases, alter credentials/permissions or connect production systems without the required user authorization.

## Scheduled maintenance

- Follow TASKS.md dependencies and resume unfinished work before selecting one new small item. Multiple runs may be needed for one item; no useful change means no commit.
- Use codex/ prefixed branches. User-authorized maintenance may push scoped, verified work to this repository's improvement branch for review, but must not automatically merge or push scheduled changes directly to main.
- Direct user-requested initialization changes may follow the explicitly approved submission path; that exception does not authorize future automatic main updates.
- Never write to upstream or another user's repository under this maintenance authorization. Do not automatically sync all upstream changes or modify the personal profile repository.
- Keep STATUS and TASKS consistent with actual results. A pushed branch is review, not done; done requires acceptance and inclusion in main. Record branch/commit, checks, limitations and next action without exposing sensitive logs.
- Commits use actual completion time and accurately describe one logical change. Follow the applicable commit skill when available; prefer type(scope): concise action, with a body for nontrivial changes.

The inherited detailed workflow and verification references are in [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md). Documentation subtree rules are in [docs/AGENTS.md](docs/AGENTS.md). Review their commands and destinations against current code before use.

## Keep this file stable

Do not put task progress, test counts, provider availability, local absolute paths or temporary incidents here. Keep changing facts in STATUS.md, TASKS.md or the relevant runbook. Product-level WorkItem acceptance rules remain distinct from repository task tracking.

## Agent skills and issue tracking

The target issue tracker, when enabled, is ggggggtm/ZoriDesk. If Issues are unavailable, use TASKS.md and report the limitation; do not enable settings or file issues in upstream without authorization. The gh CLI is optional and must not be assumed installed or authenticated.

Inherited docs/agents/issue-tracker.md provides workflow reference, not permission to write to its old upstream target. Similarly, docs/agents/triage-labels.md describes needs-triage, needs-info, ready-for-agent, ready-for-human and wontfix; verify actual labels before using them and do not create labels solely to satisfy the old guide.

For domain context use root CONTEXT.md when present and the relevant docs/adr/ entries; docs/agents/domain.md is an on-demand reference.
