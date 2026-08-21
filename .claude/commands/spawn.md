---
description: Spawn a new moomux session (worktree + tmux + agent) for a delegated task
allowed-tools: Bash(moomux spawn:*), Bash(git remote:*), Bash(basename:*)
argument-hint: <free-text task description> [project]
---

Parse `$ARGUMENTS` as a free-text task description, optionally followed by a
project name at the end if it doesn't match the current repo. Treat it as
literal text — don't try to resolve `#N` or similar tokens against GitHub
issues/PRs or anything else.

**Project**: run `moomux spawn -list` and match the current repo (e.g.
`basename $(git rev-parse --show-toplevel)`, or `git remote get-url origin`)
against a listed project name. If `$ARGUMENTS` explicitly names a different
project, use that instead. If nothing matches, ask rather than guessing.

**Task**: write a clear, self-contained prompt from `$ARGUMENTS` (the new
session's agent starts with no context beyond what you pass in `-prompt`).

**Name**: derive a short kebab-case session name from the task description.

Then run:

    moomux spawn -project <project> -name <name> -prompt "<task prompt>"

This is fire-and-forget — it creates the worktree/branch, tmux session, and
agent, types the prompt in, and returns immediately. Don't wait on or try to
check the spawned session's progress.
