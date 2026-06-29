# Shared agent config

One source of truth for what every agent on agentbox can do. The
`agentbox.yaml` playbook fans this out to both lanes:

| You add...        | Where                                   | Lands on the box                                            |
|-------------------|-----------------------------------------|-------------------------------------------------------------|
| A skill           | `skills/<name>/SKILL.md`                | `~/.claude/skills/` and `~/.config/opencode/skills/`        |
| A slash command   | `commands/<name>.md`                    | `~/.claude/commands/` and `~/.config/opencode/commands/`    |
| A script/tool     | `bin/<name>` (chmod +x)                 | `/usr/local/bin/` (on `$PATH` for both lanes)               |
| Shared guidance   | `AGENTS.md`                             | opencode `instructions`; imported into `~/.claude/CLAUDE.md`|
| An MCP server     | `agentbox_mcp_servers` in `agentbox.yaml` | opencode `mcp` block; `claude mcp add-json --scope user`  |

MCP is the one item kept in the playbook var rather than this tree: the two
tools take different MCP schemas and Claude's MCP lives in a stateful file, so a
neutral var is rendered into each. Everything else is a drop-in file here.

## Propagation and reload

Adding something = drop the file (or edit the var), then deploy.

- The autonomous lanes (`opencode run`, `claude -p`) start a fresh process per
  issue, so they pick up new skills/commands/scripts/MCP on the **next tick** with
  no reload.
- Long-lived `claude remote-control` sessions load skills/MCP/CLAUDE.md at
  startup, so the deploy `try-restart`s any active RC session to pick them up.
  That drops whatever phone session was attached; it reconnects.

## Notes

- `claude mcp` and the opencode skills dir are version-dependent surfaces; sanity
  check after a tool upgrade.
- `.gitkeep` files just keep the empty dirs tracked; harmless on the box.
