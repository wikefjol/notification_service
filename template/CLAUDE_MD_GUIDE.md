# CLAUDE.md Template Guide

## Purpose of This Document

You have a CLAUDE.md template (`CLAUDE.md.template`) and multiple existing projects with organically grown CLAUDE.md files. This guide explains:

1. What each section of the template is for and why it exists
2. How to instruct a coding agent to migrate an existing CLAUDE.md into the template structure
3. Where displaced content should go (not deleted — relocated)

---

## Philosophy

The CLAUDE.md is read by Claude Code at the start of **every session**. It becomes part of the system prompt. This has two implications:

- **Everything in it costs tokens on every interaction.** Bloated CLAUDE.md = wasted budget + diluted attention.
- **It's the single most reliable way to shape agent behavior.** Rules here are followed more strictly than ad-hoc prompts.

Boris Cherny (head of Claude Code at Anthropic) keeps his team's shared CLAUDE.md at ~2.5k tokens. The community consensus: **50-100 lines in root CLAUDE.md, with `@` pointers to detailed docs elsewhere.** Think of it as the index, not the encyclopedia.

### The "Point, Don't Dump" Principle

If you catch yourself writing more than 3-4 lines about any single topic in CLAUDE.md, that content belongs in `docs/`. Add a one-line pointer instead. The agent can read those files when it needs them; it doesn't need them pre-loaded every session.

---

## Section-by-Section Explanation

### `# [Project Name]` + One-Liner

**What**: Project identity. One sentence.
**Why**: The agent needs immediate orientation. "This is a Django REST API for university publication search" tells it what kind of decisions to make. Without this, the agent infers from file structure alone and may guess wrong.
**Token budget**: 1-2 lines.

### `## Commands`

**What**: Exact shell commands for dev, test, lint, build.
**Why**: The agent will run these. If they're wrong or missing, it guesses — and guesses wrong. This is the single highest-ROI section. Boris Cherny calls this "the commands you type repeatedly."
**Token budget**: 4-8 lines.
**Rule**: Only include commands the agent will actually execute. Don't document deployment pipelines the agent can't trigger.

### `## Architecture`

**What**: Stack summary + directory tree with annotations. Includes the GitHub repo identifier.
**Why**: Orients the agent before it starts reading files. The tree tells it where to look; the summary tells it what mental model to use. The `Repo:` line is critical — without it, the agent can't run `gh issue view` or any GitHub CLI commands because it doesn't know what repo it's in. A FastAPI project and a Django project with identical directory names behave completely differently.
**Token budget**: 5-15 lines (tree included).
**Rule**: Only show top 2 levels of tree. Deeper structure goes in `docs/specs/`.

### `## Code Style`

**What**: Language version, imports, naming, error handling.
**Why**: Prevents the agent from writing valid but inconsistent code. "Use pathlib over os.path" prevents a PR with mixed styles. "Type hints on all function signatures" prevents untyped code sneaking in.
**Token budget**: 4-8 lines.
**Rule**: Only include conventions the agent would get wrong without being told. Don't restate language defaults.

### `## Workflow`

**What**: The sequence of steps the agent should follow for any task. Includes checking GitHub issues for context.
**Why**: Without this, the agent jumps straight to implementation. With this, it reads ADRs first, checks the issue for context, proposes a plan, makes small diffs, and runs tests — because you told it to. The `gh issue view` step connects the agent to your issue tracker so it doesn't work from incomplete information.
**Token budget**: 4-6 lines.
**Rule**: Keep it to the default workflow. Task-specific workflows belong in slash commands (`/.claude/commands/`).

### `## Testing`

**What**: Framework, how to run single tests, coverage expectations, what must be tested.
**Why**: The agent will write tests. If you don't specify how, it writes tests that pass but don't verify anything meaningful (the "cycle of self-deception" problem). Stating "all repository methods must have integration tests" is more useful than "write good tests."
**Token budget**: 3-6 lines.

### `## Boundaries`

**What**: Hard rules using `YOU MUST NOT` and `IMPORTANT` markers.
**Why**: Research and community experience confirm that `YOU MUST NOT` and `IMPORTANT` markers measurably improve compliance. This is where you prevent the agent from touching auth code, adding random dependencies, or skipping tests. These are your guardrails.
**Token budget**: 3-6 lines.
**Rule**: If everything is marked IMPORTANT, nothing is. Reserve this for rules where violation causes real damage.

### `## Context Pointers`

**What**: One-line references to detailed documentation files.
**Why**: This is the "point, don't dump" section. Instead of explaining your API conventions in CLAUDE.md (eating tokens every session), you write `API conventions: docs/api.md` and the agent reads it when relevant.
**Token budget**: 3-8 lines (just paths and labels).
**What goes in `docs/`**:
- `docs/adr/` — Architectural Decision Records (see ADR guide from research)
- `docs/specs/` — Component and feature specifications
- `docs/runbooks/` — Operational procedures (deploy, rollback, data migration)
- `docs/setup.md` — Environment setup, dependencies, secrets management
- `docs/api.md` — API conventions, endpoint patterns, auth flows

### `## Current Focus`

**What**: Living section updated at session boundaries. Uses a structured format:
- **Active**: issue number + description, with three sub-lines:
  - **Next**: a concrete task the agent can start immediately (no recon needed)
  - **Done when**: specific criteria so the agent knows when to stop
  - **Issue**: full URL or `gh issue view N` so the agent can get more context if needed
- **Last session**: what was accomplished
- **Next up**: what comes after the active work

**Why**: This is your **session handoff document**. It solves the cold-start problem. Without "Next" and "Done when", an agent sees "Active: #50 documentation consolidation" and has to run recon before it can start. With them, zero round-trips — the agent begins working immediately.
**Token budget**: 4-8 lines.
**Rule**: Update this at the end of every working session. It's a habit, not a one-time setup.
**Automation tip**: You can create a slash command (`/.claude/commands/session-end.md`) that prompts the agent to update this section before wrapping up.

### `## Gotchas`

**What**: Non-obvious things that waste time.
**Why**: Every project has them. "The Stripe webhook handler must validate signatures" saves hours of debugging. "Tests require Docker running" prevents mysterious CI failures. This is institutional knowledge.
**Token budget**: 2-5 lines.
**Rule**: Add a gotcha every time the agent (or you) gets bitten by something. This is the CLAUDE.md equivalent of Boris's "common issues spreadsheet."

---

## Migration Strategy

You have existing CLAUDE.md files with valuable content in non-standard layouts. Here's how to migrate without losing anything.

### The Prompt

Copy this into Claude Code when you're ready to migrate a project. Adapt paths as needed.

```
I'm migrating this project's CLAUDE.md to a standardized template. Here's what I need you to do:

1. Read the template at [path/to/CLAUDE.md.template] (or I'll paste it).
2. Read the current CLAUDE.md in this project.
3. For each piece of content in the current CLAUDE.md, determine which template section it belongs to. If it doesn't fit any section, it probably belongs in docs/.
4. Create the new CLAUDE.md following the template structure, migrating content into the correct sections.
5. For content that's too detailed for CLAUDE.md (more than 3-4 lines on a single topic), create the appropriate file in docs/ and add a pointer in the Context Pointers section.
6. DO NOT delete any information. Everything must either be in the new CLAUDE.md or in a docs/ file that's referenced from it.
7. Show me a summary of what moved where before committing.

Rules:
- Keep each CLAUDE.md section within its token budget (see comments in template).
- Preserve exact commands, paths, and technical details — don't paraphrase them.
- If you're unsure where something goes, put it in docs/ and point to it. Better to over-document than to lose information.
- Update the Current Focus section based on git log and any TODO/FIXME comments in the codebase.
```

### Migration Mapping Cheat Sheet

| If the old CLAUDE.md has... | It goes to... |
|---|---|
| Shell commands, how to run things | `## Commands` |
| Tech stack description, framework info | `## Architecture` (summary) or `docs/specs/` (detail) |
| Directory structure | `## Architecture` (top 2 levels) |
| Coding conventions, style rules | `## Code Style` |
| "Don't touch X", "Never do Y" | `## Boundaries` |
| Detailed API docs, schemas | `docs/api.md` → pointer in `## Context Pointers` |
| Implementation plans, roadmaps | `docs/specs/` → pointer in `## Context Pointers` |
| Setup instructions, env vars | `docs/setup.md` → pointer in `## Context Pointers` |
| Design decisions, "we chose X because Y" | `docs/adr/` → pointer in `## Context Pointers` |
| Current status, what's done/not done | `## Current Focus` |
| Bugs, workarounds, "watch out for" | `## Gotchas` |
| Workflow descriptions | `## Workflow` (if default) or `/.claude/commands/` (if task-specific) |
| Test instructions | `## Testing` |
| Everything else | `docs/` with a pointer |

### Post-Migration Checklist

After migration, verify:

- [ ] `CLAUDE.md` is under ~100 lines / ~2.5k tokens
- [ ] Every `docs/` pointer actually points to an existing file
- [ ] Commands section has runnable commands (test them)
- [ ] Boundaries section uses `YOU MUST NOT` / `IMPORTANT` markers
- [ ] Current Focus is up to date
- [ ] `docs/adr/INDEX.md` exists (even if empty, with a header explaining what goes here)
- [ ] No detailed prose remains in CLAUDE.md that should be in docs/
- [ ] Git commit the migration as a single commit: `docs: migrate CLAUDE.md to standardized template`

---

## The `docs/` Directory Structure

After migration, your docs/ should look something like:

```
docs/
├── adr/
│   ├── INDEX.md          # Auto-generated or manually maintained list
│   ├── 001-use-fastapi.md
│   └── 002-postgres-over-sqlite.md
├── specs/
│   ├── component-x.md
│   └── feature-y.md
├── runbooks/
│   └── deploy.md
├── setup.md              # Environment, dependencies, secrets
└── api.md                # API conventions (if applicable)
```

ADR files should use YAML frontmatter with `scope.paths` so the agent knows which ADRs to read before modifying which files. See the ADR research notes for the full schema.

---

## Maintenance Habits

1. **End of session**: Update `## Current Focus`. Make this a reflex, or use a slash command.
2. **After a PR review catches something**: Add it to `## Gotchas` or `## Boundaries`.
3. **After an architectural decision**: Create an ADR in `docs/adr/`.
4. **Monthly**: Re-read CLAUDE.md. Cut anything that's no longer true. Move anything that's grown too long into docs/.

---

## Integration with Your Hooks Setup

Your current hooks config (Notification + Stop → submarine sound via `afplay`) already handles the "agent needs you" case. When you get your central webhook notification system running, you can swap the `afplay` command for a `curl` to your webhook endpoint, and it'll work identically over SSH:

```json
{
  "hooks": {
    "Notification": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "curl -s http://your-webhook-host:port/notify -d '{\"source\":\"claude-code\",\"event\":\"attention\"}' || afplay /System/Library/Sounds/Submarine.aiff -v 10"
          }
        ]
      }
    ]
  }
}
```

The `||` fallback means: try webhook first, fall back to local sound if webhook is unreachable. Clean migration path from local-only to your owned notification system.

---

## Quick Reference: Token Budget

| Section | Target Lines | Purpose |
|---|---|---|
| Header + one-liner | 2 | Orient |
| Commands | 4-8 | Execute |
| Architecture | 5-15 | Navigate |
| Code Style | 4-8 | Conform |
| Workflow | 4-6 | Sequence |
| Testing | 3-6 | Verify |
| Boundaries | 3-6 | Constrain |
| Context Pointers | 3-8 | Discover |
| Current Focus | 4-8 | Continue |
| Gotchas | 2-5 | Survive |
| **Total** | **~50-80** | **~2-2.5k tokens** |
