# Maintainers

This page names the roles that decide things in Margince and how to reach each one. A name is added
next to a role only after its owner agrees to it. Who reviews which path is a separate question, and
[CODEOWNERS](CODEOWNERS) answers it.

| Role | Decides | How to reach it |
|---|---|---|
| Product owner | What Margince does next, and any question the code and docs leave open | Open an issue and ask for the `status: needs-decision` label |
| Maintainers (`@margince/margince`) | Whether a pull request merges, and how the code is built | A pull request; the default code owner is asked for review |
| Extension tier owner | The `backend/pkg/` surface and the rules an extension must follow | A pull request under the paths [CODEOWNERS](CODEOWNERS) gives that owner |
| Security contact | How a reported hole is judged and fixed | In private, as [SECURITY.md](SECURITY.md) says; never a public issue |
| Conduct contact | Reports under the [Code of Conduct](CODE_OF_CONDUCT.md) | `conduct@gradion.com` |

## Where Margince is going

The public [Margince roadmap](https://github.com/margince/margince/projects) groups issues by their
milestone into **Now**, **Next** and **Later**. A maintainer brings it up to date by hand, so it can lag
behind. An issue's own milestone and state are the source of truth, and the board only sorts them.
