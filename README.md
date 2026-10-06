<!-- prose:plain -->
# Margince

Margince is a CRM that fills its own records from mail, meetings and calendars. AI agents work in it under
the same rules as its users.

- **It fills itself.** Mail and meetings from the accounts you connect become contacts, companies and deals.
- **Every AI change shows its source.** You can open a forecast number and read the deals behind it.
- **Agents use the same engine as users.** An agent connects over MCP or REST. It never has more rights
  than the user who gave it access.
- **You change it in code.** There are no setup screens. A custom field or a workflow is a normal code
  change in your own copy.
- **It runs where you choose.** Your own servers with Docker, a private cloud, or one folder on a laptop.

The product site is [margince.com](https://margince.com). This repository holds the source code.

## Use it

The [handbook](docs/handbook/README.md) explains the app for the teams who use it. It has no code.

To run your own copy, start with [docs/deployment.md](docs/deployment.md).

## Run it on your machine

You need Go 1.27 or later, Docker with Compose, `jq`, `golangci-lint`, Node 24 and pnpm.

1. Install the tools and the git hooks once: `make install`.
2. Start the stack: `make dev`. It starts Postgres, Redis, the API and the web app, then returns.
3. Add demo data: `make seed-dev`.
4. Open http://localhost:8080 and sign in as `admin@demo.test` with `demo-password-123`.
5. Check that it all works: `make verify-boot`.

Stop it with `make dev-stop`. Without step 3 the app starts empty, the way a new customer sees it. The
first admin password is then in `config/margince-admin-password`, and the app asks you to change it.

Each git worktree gets its own database and ports, so two copies can run at the same time.
[docs/reference/make-targets.md](docs/reference/make-targets.md) lists every command.

## Work on the code

- [CONTRIBUTING.md](CONTRIBUTING.md): how to send a change.
- [AGENTS.md](AGENTS.md): the rules every change must follow, for humans and coding agents.
- [docs/explanation/backend-onboarding.md](docs/explanation/backend-onboarding.md): a map of the backend
  and the order to read it in.
- [docs/README.md](docs/README.md): all the documentation.

`make check` is the test that every pull request must pass. Run it before you push.

The code is one Go module in `backend/` and a React app in `frontend/`. The API contract is
`backend/api/crm.yaml`. The server code and the web app's types are built from it, so the two cannot
disagree.

## How agents are kept safe

- An agent works with a passport that a user gives it. Every call checks that user's seat and rights,
  so if you remove the user at 09:00, the agent stops at 09:00.
- An agent cannot approve its own work. Approvals, consent, data requests and pipeline settings are
  closed to agents.
- Every action goes into a log that the app can add to but not change.
- Each passport has a daily limit on how much it can send.
- Most actions run at once. A smaller set waits for a user to approve it.
  [docs/reference/agent-tools.md](docs/reference/agent-tools.md) lists which is which.

## Where your data goes

Each AI task can use a model on your own machines (Ollama or vLLM) or a cloud provider. A task that uses
a cloud provider sends the text it reads to that provider. The `sovereign` profile refuses every cloud
provider. [docs/reference/ai-egress.md](docs/reference/ai-egress.md) lists, for each task, what can leave.

## What is not here yet

- Outbound email sequences that send with no one watching. An agent can write a draft for a user to send.
- Phone calls and click-to-call.
- A hosted service or many companies in one installation. One installation serves one company.

Open work is in [GitHub issues](https://github.com/margince/margince/issues), and past releases are in
[CHANGELOG.md](CHANGELOG.md).

## Security

Report a security problem in private, as [SECURITY.md](SECURITY.md) explains. Do not open a public issue
for it.

## License

Margince uses the Business Source License 1.1 ([LICENSE](LICENSE)). The source is public. You may read,
run and change it.

- You may use it free for your own company, up to 10 seats. A seat is a named user. AI agents and
  service accounts are not seats.
- To host it as a service for other companies, you need a hosting partner agreement with Gradion.
- Each release becomes Apache 2.0 two years after it ships.

Outside `dev` and `test` mode, the server needs a license token from Gradion to start, even for one
seat. Today you ask us for that token. If this blocks you, please say so in an issue.

Prices are on [margince.com](https://margince.com). Margince is built by [Gradion](https://gradion.com).
