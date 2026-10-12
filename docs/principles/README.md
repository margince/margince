<!-- prose:plain -->
# Principles

How this repository decides things, set down in one place so a reader does not have to work it out
from the code.

A principle states one thing that is true about the shape of this code, and how to check that the
tree still has it. Each one decides a kind of question before someone has to ask it.

These pages explain; they do not make a rule hold. The rules a change must follow are in the
rulebook, `AGENTS.md`. The gates that hold these rules are tests and the craftsmanship gate. The
rulebook links down to these pages, and these pages do not link up to it. So when the rulebook
renames a section, no link here can break. When a principle here and a gate disagree, the gate
shows what the code does today: fix one or the other, and say which.

| Principle | What it decides | Rulebook section it explains |
|---|---|---|
| [One source of truth](one-source-of-truth.md) | Where a question is decided, why a second implementation is a bug and not only a waste, and which module line the owner may be behind. Holds the scan to audit a part of the system. | *Reuse before you build*, *Layout* |
| [The record is the code](the-record-is-the-code.md) | Which source decides when two sources disagree, and where a finding goes once you have it. | *What decides a question* |
| [Every mutation leaves a trace](every-mutation-leaves-a-trace.md) | Why the domain row, the audit row and the event commit together, and what each of the two denials means. | *The write shape* |
| [Code a reader can follow](legibility-is-the-product.md) | Why craftsmanship is a gate and not what a reviewer likes, and what harm each anti-tell keeps out. | *Craftsmanship* |
| [Derive the obligation](derive-the-obligation.md) | Why a gate must hold a rule, not a human, and how to write a gate that holds instead of one that shows green over its own bug. It also covers the two ways a gate becomes the second copy it exists to stop. [reference/gate-patterns.md](../reference/gate-patterns.md) lists the shapes a gate comes in, and [reference/gate-inventory.md](../reference/gate-inventory.md) is the generated list of every gate. | *Rules learned from the review loop* |
| [Nothing here is private](nothing-here-is-private.md) | Who the public reader is, what never shows in the tree, and why a working attack takes the private path. | *This repository is public* |

## Adding a principle

Write one only when the same question has come up more than once. If no past change could end
some other way under it, it is only what someone likes. Put that in the rulebook or leave it out.

Each page has the principle, the way to check it, and what the principle does *not* ask for. The
last part keeps a principle from being used past its reach.
