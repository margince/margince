# Maintainers

This page names the roles that decide things in Margince and how to reach each one. It names roles,
not people: a person is added next to a role only after they agree to be named. Who reviews which
path is a separate question, and [CODEOWNERS](CODEOWNERS) answers it.

| Role | Decides | How to reach it |
|---|---|---|
| Product owner | What Margince does next, and any question the code and docs leave open | Open an issue and ask for the `status: needs-decision` label |
| Maintainers (`@margince/margince`) | Whether a pull request merges, and how the code is built | A pull request; the default code owner is asked for review |
| Extension tier owner | The `backend/pkg/` surface and the rules an extension must follow | A pull request under the paths [CODEOWNERS](CODEOWNERS) gives that owner |
| Security contact | How a reported hole is judged and fixed | In private, as [SECURITY.md](SECURITY.md) says; never a public issue |
| Conduct contact | Reports under the [Code of Conduct](CODE_OF_CONDUCT.md) | `conduct@gradion.com` |

## Where Margince is going

The public [Margince roadmap](https://github.com/orgs/margince/projects/2) sorts open issues with a
dated milestone into **Now**, **Next** and **Later**. The issue is the source of truth, and the project
only sorts it.
