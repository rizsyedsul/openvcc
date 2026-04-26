# Instructions for AI Coding Agents

These rules apply to any AI coding agent (Claude Code, Cursor, etc.) working in this repository. They exist to keep work productive on long sessions and recoverable when the harness misbehaves.

## Output discipline

- No single `Write` call should exceed ~150 lines. Build large files incrementally with multiple `Edit` calls.
- For long docs or plans, emit one section per turn; wait for "continue" before the next.
- Prefer focused `Read` calls (`offset` / `limit`) over reading whole files when only a section is needed.
- Don't echo huge tool outputs back into the thread — let them sit in the tool result.

## Tool / agent use

- Background `Task` agents that finish mid-turn are a known stall trigger. Prefer foreground subagents unless parallelism is essential.
- After any tool call, continue immediately. Don't sit idle.
- When you call multiple tools with no dependencies between them, batch them in a single message.

## Context hygiene

- When context is > ~70% full, run `/compact` before starting a new long task.
- For multi-file work: short plan first, then file-by-file.

## Bedrock-specific (only if `CLAUDE_CODE_USE_BEDROCK=1`)

- Lower `maxOutputTokens` to 16k–32k. The default 64k is the most common cause of mid-stream stalls on Bedrock.

## On a stall

1. `claude --version` — must be ≥ 2.1.105.
2. Check status.claude.com before reauthing or reinstalling.
3. Recover partial output from `~/.claude/projects/.../*.jsonl` (grep the last assistant message).
4. Resume with: "Continue from this last partial line: <paste>".
5. If reproducible, file via `/feedback` with the `request_id`.

## Commit cadence

Commit after every logical milestone — one package, one IaC module, one example app. Small commits make stall recovery cheap and keep the history reviewable.

## Naming

- Project display name: **Open VCC**
- Slug / Go module / binary / Docker image / config file: **`openvcc`**
- Runtime component: the **engine**. CLI: `openvcc engine serve`.
- "Connector" is the umbrella concept; "engine" is the running process. Never mix them.
