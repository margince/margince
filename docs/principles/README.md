# Principles

How this repository decides things, written down once so a reader does not have
to reconstruct it from the code.

A principle here is broader than a rule and narrower than a philosophy: it is a
statement about the shape of this codebase that settles a class of arguments
before they start, plus the method for checking whether the tree still holds it.

These pages explain; they do not enforce. The binding rules live in `AGENTS.md`
at the repository root, and the gates that hold them live in tests and in
`cli/craft`. The rulebook links down to these pages; they do not link back up to
it, so a heading it renames cannot leave a dead anchor here. When a principle
here and a gate disagree, the gate is the record of current behaviour — fix one
or the other, and say which.

| Principle | It settles | Rulebook section it explains |
|---|---|---|
| [One source of truth](one-source-of-truth.md) | Where a topic is decided, why a second implementation is a defect rather than untidiness, and which module boundary the owner may live behind. Carries the six-probe scan for auditing a subsystem. | *Reuse before you build*, *Layout* |
| [The record is the code](the-record-is-the-code.md) | What outranks what when two sources disagree, and where a finding goes once you have it. | *What decides a question here* |
| [Every mutation leaves a trace](every-mutation-leaves-a-trace.md) | Why the domain row, the audit row and the event commit together, and what each of the two denials means. | *The write shape* |
| [Legibility is the product](legibility-is-the-product.md) | Why the craft bar is a gate rather than taste, and what each anti-tell is defending against. | *Craftsmanship* |
| [Derive the obligation](derive-the-obligation.md) | Why a rule is held by a gate rather than by memory, and how to write one that actually holds rather than one that reads green over its own defect — including the two ways a gate becomes the duplicate it forbids. The shapes a gate comes in are cataloged in [reference/gate-patterns.md](../reference/gate-patterns.md), and the gates themselves are generated into [reference/gate-inventory.md](../reference/gate-inventory.md). | *Rules learned from the review loop* |
| [Nothing here is private](nothing-here-is-private.md) | Who the public reader is, what never appears in the tree, and why a working exploit takes the private path. | *This repository is public* |

The rulebook sections stay where they are. `cli/craft` feeds the nearest
`AGENTS.md`'s **`## Craftsmanship` section** into its gate prompt, so a rule
moved out of that section stops reaching the gate — these pages carry the
reasoning and the method, the rulebook carries the binding short form. Every `CLAUDE.md` holds nothing but an
`@AGENTS.md` import, so no rule has a second copy to drift from. A directory may
carry its own `AGENTS.md` for rules that bind only inside it — `frontend/` does —
and those only ADD: a rule binding the whole tree lives in the root rulebook and
nowhere else.

## Adding a principle

Write it only when the same argument has been had more than once. A principle
that no past change would have gone differently under is not a principle, it is
a preference — put that in the rulebook or leave it out.

Each page carries three things: the statement, the method for checking it, and
what the principle explicitly does NOT ask for. The third is what stops a
principle being applied as a cudgel.
