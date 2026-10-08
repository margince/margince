# Glossary

Each technical name a plain page may use, with what it means. A plain page uses general words from its
area's word list and the names below. A name matches only as written here.

To add a name, add a row with a meaning of at least three words. An ordinary English word does not belong
here: put it in the area's word list instead. A name that no plain page uses must leave this table.

| Term | Meaning |
|---|---|
| AC | Acceptance criterion: a numbered screen check, as in `AC-<screen>-N`. |
| AES | The Advanced Encryption Standard, used as AES-256-GCM to seal secrets. |
| AI | Artificial intelligence: the language models Margince calls. |
| Apache | The Apache License 2.0, which each release becomes after two years. |
| API | The HTTP interface the server offers, defined in `backend/api/crm.yaml`. |
| backend | The Go server code under `backend/`. |
| BUSL | The Business Source License 1.1, the license Margince uses. |
| changelog | The list of changes in each release, kept in `CHANGELOG.md`. |
| CI | Continuous integration: the checks GitHub runs on every pull request. |
| Compose | Docker Compose, which starts Postgres and Redis for local work. |
| Conventional | Conventional Commits, a format for commit messages. |
| Corepack | The Node tool that installs the pinned version of pnpm. |
| CRA | The EU Cyber Resilience Act, which sets duties for software makers. |
| CRM | Customer relationship management: software for contacts, companies and deals. |
| CSV | Comma-separated values, a plain text format for tables. |
| dev | The development mode of a local install, set by `MARGINCE_ENV`. |
| Diátaxis | A way to sort docs into tutorials, how-to guides, reference and explanation. |
| Docker | The container tool used to run the local stack and the images. |
| English | The language the docs and the source catalog are written in. |
| EU | The European Union, whose laws several pages refer to. |
| GDPR | The EU General Data Protection Regulation, the law on personal data. |
| German | The language of the German compliance pack and UI catalog. |
| Germany | The country the German compliance pack is written for. |
| git | The version control tool the repository uses. |
| GitHub | The site that hosts the repository, issues and pull requests. |
| Gmail | Google's mail service, one of the mailboxes Margince connects to. |
| Go | The programming language of the server. |
| Google | The company behind Gmail and the Gemini models. |
| Gradion | The company that builds Margince and licenses it. |
| HNSW | A vector index type, Hierarchical Navigable Small World, that Margince does not use. |
| HTTPS | HTTP over an encrypted connection. |
| IMAP | The standard protocol for reading mail from a mail server. |
| LICENSE | The file at the repository root that holds the license text. |
| LinkedIn | The professional network whose contact export Margince can import. |
| macOS | Apple's desktop operating system. |
| Margince | The CRM this repository builds. |
| MCP | Model Context Protocol: how an AI agent connects to Margince's tools. |
| Microsoft | The company behind Microsoft 365 mail and calendars. |
| Node | Node.js, the JavaScript runtime the web app's tools run on. |
| Ollama | A tool that runs language models on your own machine. |
| OpenRouter | A service that routes model calls to many AI providers. |
| pnpm | The package manager for the web app. |
| Postgres | PostgreSQL, the database Margince stores its data in. |
| RBAC | Role-based access control: what each role may see and change. |
| React | The JavaScript library the web app is built with. |
| Redis | The in-memory store Margince uses as its event bus. |
| REST | The style of HTTP API Margince offers next to MCP. |
| SonarCloud | A code quality service that scans each pull request. |
| Telegram | A chat app Margince can send messages through. |
| UTF | The Unicode text encoding family, as in UTF-8. |
| VAT | Value added tax, and the tax number a company is registered under. |
| Vite | The build and dev server tool for the web app. |
| vLLM | A server that runs language models on your own machines. |
| Windows | Microsoft's desktop operating system. |
| Worklist | The app's list of tasks and items that need a user's action. |
| worktree | A second git checkout of the repository with its own branch. |

## Names without a meaning yet

The pages used these names before they joined the plain-words bar. Move a name up into the table when you
write its meaning, and delete it here. A name new to the docs goes in the table, never here.

<!-- prose:allow sentence a list of names, not a sentence -->
`AA`, `AAAA`, `ack`, `acks`, `AD`, `AirDrop`, `AkashML`, `allowlist`, `American`, `Android`, `Anthropic`, `AO`,
`api`, `apidiff`, `Apple`, `arch-lint`, `args`, `ARIA`, `Art`, `ASCII`, `AST`, `Atlassian`, `Aurora`,
`Austria`, `Austrian`, `auth`, `Authenticode`, `Auto-capture`, `auto-capture`, `axe`, `Azure`, `B`, `BaseTen`,
`bash`, `Bcc`, `BDSG`, `Betriebsrat`, `Betriebsvereinbarung`, `BetrVG`, `BI`, `Biome`, `biome`, `BotFather`,
`British`, `BSD`, `Buchungsbelege`, `BYOK`, `BYPASSRLS`, `C`, `CA`, `calendarView`, `Cc`, `CDN`, `Cerebras`,
`cert`, `CGNAT`, `ChatGPT`, `CHF`, `chi`, `Chrome`, `Chromium`, `chunker`, `Claude`, `claude`, `CLDR`, `CLI`,
`Cloud`, `Cmd`, `CNAME`, `codegen`, `CODEOWNERS`, `CodeRabbit`, `Codex`, `codex`, `comms`, `compose`, `config`,
`contrib`, `CoreWeave`, `cosign`, `Covenant`, `CPU`, `craft`, `CredentialRotator`, `CredentialSink`, `cron`,
`CRUD`, `CSRF`, `CSS`, `Ctrl`, `Ctrl-C`, `curl`, `CV`, `CycloneDX`, `D`, `DACL`, `DAG`, `DATEV`, `DB`, `db`,
`DCR`, `DDL`, `dedupe`, `DeepInfra`, `DeepSeek`, `DELETE`, `depguard`, `deps`, `Desktop`, `Deutsch`, `dist`,
`DKIM`, `DLL`, `DLLs`, `DMARC`, `DML`, `DNA`, `DNS`, `Dockerfile`, `DocuSign`, `DOM`, `dorny`, `DPA`, `DPIA`,
`DSFA`, `DSGVO`, `DSL`, `DSN`, `DSNs`, `DSR`, `DTO`, `EAV`, `Edge`, `Einwilligung`, `embeddings`,
`Empfangsbestätigung`, `Enterprise`, `Entra`, `entrypoint`, `enum`, `env`, `ERP`, `Esc`, `Escape`, `ETag`,
`EUR`, `European`, `evaluator`, `Excel`, `Exchange`, `FAQ`, `Fastmail`, `favicons`, `FE`, `Firefox`,
`Flash-Lite`, `Fonts`, `forbidigo`, `Forrester`, `frontend`, `FX`, `Garnet`, `Gartner`, `Gatekeeper`, `GB`,
`GBP`, `gcal`, `GCM`, `Geist`, `Gemini`, `Gemma`, `Geocoding`, `GGUF`, `GH`, `GiB`, `gitignored`, `gitleaks`,
`glob`, `GNU`, `go-arch-lint`, `GoBD`, `gofmt`, `golangci`, `golangci-lint`, `gosec`, `govulncheck`, `GPL`,
`GPT`, `GPU`, `Graph`, `graphcal`, `grep`, `Groq`, `GUC`, `Haiku`, `Handelsbrief`, `HardPass`, `HGB`, `HMAC`,
`HMAC-SHA`, `HTML`, `HTTP`, `Hà`, `IAM`, `IANA`, `ICP`, `ID`, `id`, `idempotency`, `Idempotency-Key`, `IDs`,
`IMAPS`, `Impressum`, `Inspector`, `Intel`, `Intelligence`, `IP`, `iPhone`, `ISO`, `Jev`, `jq`, `jsdom`,
`JSON`, `JSON-LD`, `JSON-RPC`, `JSONL`, `JSONPath`, `K`, `KB`, `Kev`, `Keychain`, `keyvault`, `Lars`, `Laya`,
`lcov`, `Levenshtein`, `libpq`, `Linux`, `LiteLLM`, `LLM`, `LOC`, `localhost`, `lockfile`, `London`, `Mac`,
`Mach-O`, `MAJOR`, `Makefile`, `Markdown`, `markdown`, `MB`, `Meet`, `MFA`, `MiB`, `micro-USD`, `middleware`,
`MIME`, `Minh`, `MinIO`, `Ministral`, `MINOR`, `Mistral`, `MIT`, `Mitarbeiterinformation`, `MLX`, `MoE`, `MRL`,
`ms`, `MSVC`, `mtime`, `MX`, `NAT`, `Nebius`, `Nemo`, `NFC`, `NL`, `non-match`, `nonce`, `NormalizedRecord`,
`notarization`, `Novita`, `npm`, `Nr`, `NULL`, `null`, `OA`, `oapi-codegen`, `oasdiff`, `OAuth`, `Office`,
`OIDC`, `OpenAI`, `OpenAI-compatible`, `OpenAPI`, `Ops`, `ops`, `OS`, `Outfit`, `Outlook`, `Parasail`, `PATCH`,
`PDF`, `PERF`, `pgvector`, `pids`, `PII`, `PIM`, `Playwright`, `POSIX`, `POST`, `PostgreSQL`, `POSTs`,
`PowerShell`, `PR`, `PRs`, `PTR`, `Pub`, `purl`, `push-capable`, `px`, `Python`, `QC`, `Qwen`, `RAM`,
`re-authenticate`, `Re-certify`, `re-certify`, `README`, `Rekor`, `Renovate`, `RFC`, `River`, `RLS`, `Rosetta`,
`RPC`, `RRF`, `Safari`, `SAR`, `SBOM`, `SBOMs`, `SDR`, `semver`, `send-capable`, `SendGrid`, `SHA`, `sha`,
`Shopify`, `Shopware`, `Shortlist`, `Shortlists`, `sigstore`, `SiliconFlow`, `SKU`, `SLA`, `slug`,
`SmartScreen`, `SMB`, `SMTP`, `Sonnet`, `SPA`, `Sparkles`, `SPDX`, `SPF`, `SQL`, `SQLSTATE`, `SSRF`, `stderr`,
`stdin`, `stdout`, `storekit`, `Storybook`, `Streamable`, `Stripe`, `struct`, `structs`, `Studio`, `Sub`,
`sub-issue`, `Surface-B`, `Surfe`, `Svix`, `syft`, `SyncOnce`, `syncToken`, `T`, `TCP`, `testkit`, `ThreadKey`,
`Tiếng`, `TLS`, `Trần`, `TS`, `tsc`, `TTL`, `TUF`, `TXT`, `typecheck`, `TypeSafe`, `TypeScript`, `UA`, `UAT`,
`UI`, `UID`, `uid`, `und`, `unix`, `URI`, `URIs`, `URL`, `URL-safe`, `USB`, `USP`, `UTC`, `UUID`, `uuid`,
`Valkey`, `vCard`, `vCPU`, `Ventura`, `Verarbeitungsverzeichnis`, `Vertex`, `vet`, `VIES`, `Vietnamese`,
`Visual`, `vite`, `Vitest`, `vitest`, `Việt`, `vllm-metal`, `Voice-DNA`, `VS`, `vuln`, `WAI-ARIA`,
`Wappalyzer`, `WatchRenewer`, `WCAG`, `WebGL`, `Wettbewerbszentrale`, `worktrees`, `WSL`, `Xcode`, `XRechnung`,
`YAML`, `Zalo`, `ZUGFeRD`, `Zürich`
