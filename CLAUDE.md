@AGENTS.md

# CLAUDE.md — the Claude Code half

Everything above this line is [AGENTS.md](AGENTS.md), imported. **That file is
the rulebook** — one copy of every rule, read by every harness that works here.
Claude Code reads `CLAUDE.md` and not `AGENTS.md`, which is the only reason this
file exists: the `@AGENTS.md` on the first line is what puts the rulebook in
front of Claude.

So do not copy a rule down here. A rule stated in this file reaches Claude and
nothing else — not Codex, and not `cli/craft`, which feeds the nearest
`AGENTS.md` into its gate prompt. The two files carrying full copies of the rules
is exactly what this arrangement replaced, and
`TestClaudeMdOnlyImportsTheRulebook` in `backend/rulebookdelegation_test.go` fails
if a rule section grows back here.

What belongs below is only what is true of Claude Code and false of the other
harnesses.

## Claude Code

**Nested instruction files load on demand.** `frontend/CLAUDE.md` is not in your
context now; Claude Code pulls it in when you read a file under `frontend/`.
Working in there, open it and then the file it opens with —
[frontend/src/design-system/README.md](frontend/src/design-system/README.md), the
catalog of every control that already exists. Other harnesses do not do this, so
`AGENTS.md` names both files in its *Layout* section rather than relying on the
mechanism.

**Path-scoped rules and skills are the place for anything long.** A procedure
that matters for one part of the tree belongs in `.claude/rules/` with a `paths:`
frontmatter glob, or in a skill — not in the rulebook, which every session pays
for in full. Adding to `AGENTS.md` costs every session and every gate prompt;
that is the right price for a binding rule and the wrong one for a runbook.

**Session scratch goes in the session's own temp directory**, never a
`scratchpad/` at the repo root. The rulebook's *Never commit machine or session
debris* section is the binding form.
