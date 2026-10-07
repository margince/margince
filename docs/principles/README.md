# Principles

How this repository decides things, written down once so a reader does not have
to reconstruct it from the code.

A principle states one property of this codebase's shape, which settles a class
of arguments before they start, and how to check that the tree still has it.

These pages explain; they do not enforce. The binding rules live in `AGENTS.md`
at the repository root, and the gates that hold them live in tests and in the
craftsmanship gate. The rulebook links down to these pages; they do not link back up to
it, so a heading it renames cannot leave a dead anchor here. When a principle
here and a gate disagree, the gate is the record of current behaviour: fix one
or the other, and say which.

| Principle | It settles | Rulebook section it explains |
|---|---|---|
| [One source of truth](one-source-of-truth.md) | Where a topic is decided, why a second implementation is a defect rather than untidiness, and which module boundary the owner may live behind. Carries the scan for auditing a subsystem. | *Reuse before you build*, *Layout* |
| [The record is the code](the-record-is-the-code.md) | What outranks what when two sources disagree, and where a finding goes once you have it. | *What decides a question here* |
| [Every mutation leaves a trace](every-mutation-leaves-a-trace.md) | Why the domain row, the audit row and the event commit together, and what each of the two denials means. | *The write shape* |
| [Legibility](legibility-is-the-product.md) | Why the craft bar is a gate rather than taste, and what each anti-tell is defending against. | *Craftsmanship* |
| [Derive the obligation](derive-the-obligation.md) | Why a rule is held by a gate rather than by memory, and how to write one that holds rather than one that reads green over its own defect, including the two ways a gate becomes the duplicate it forbids. The shapes a gate comes in are cataloged in [reference/gate-patterns.md](../reference/gate-patterns.md), and the gates themselves are generated into [reference/gate-inventory.md](../reference/gate-inventory.md). | *Rules learned from the review loop* |
| [Nothing here is private](nothing-here-is-private.md) | Who the public reader is, what never appears in the tree, and why a working exploit takes the private path. | *This repository is public* |

## Adding a principle

Write it only when the same argument has been had more than once. If no past
change would have gone differently under it, it is a preference: put that in the
rulebook or leave it out.

Each page carries the statement, the method for checking it, and what the
principle does *not* ask for. The last part keeps a principle from being applied
beyond its reach.
