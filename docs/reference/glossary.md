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
`AA`, `AAAA`, `Abschlüsse`, `ack`, `Acme`, `AD`, `AirDrop`, `allowlist`, `Android`, `Anna`, `Anthropic`, `AO`,
`api`, `Apple`, `arch-lint`, `args`, `argv`, `Art`, `ASCII`, `AST`, `Atlassian`, `Aurora`, `Austria`,
`Austrian`, `auth`, `Authenticode`, `Auto-capture`, `auto-capture`, `axe`, `Azure`, `B`, `b`, `bash`, `Bcc`,
`bcc`, `BDSG`, `Betriebsrat`, `Betriebsvereinbarung`, `BetrVG`, `BI`, `Biome`, `biome`, `blobstore`, `BLOCKER`,
`BMP`, `bool`, `BotFather`, `BSD`, `Buchungsbelege`, `BuildKit`, `BYOK`, `BYPASSRLS`, `bytea`, `Bücher`, `C`,
`CA`, `Cache`, `cAdvisor`, `calendarView`, `Carryover`, `CAS`, `Cc`, `cc`, `CDN`, `cert`, `CGNAT`, `ChatGPT`,
`CHF`, `chi`, `Chrome`, `Chromium`, `chunker`, `chủ`, `CIDR`, `Claude`, `claude`, `CLDR`, `CLI`, `Cloud`,
`Cmd`, `cmd`, `CNAME`, `codegen`, `CODEOWNERS`, `CodeRabbit`, `Codex`, `codex`, `comms`, `compose`, `Conduct`,
`config`, `constellation`, `contrib`, `CPU`, `craft`, `CredentialRotator`, `CredentialSink`, `cron`, `CRUD`,
`CSRF`, `CSS`, `Ctrl`, `Ctrl-C`, `curl`, `customfields`, `CV`, `CycloneDX`, `D`, `DACHPartner`, `DACL`, `DAG`,
`DB`, `db`, `DB-less`, `DCR`, `DDL`, `dedupe`, `DeepSeek`, `DELETE`, `depguard`, `Desktop`, `Deutsch`, `Diff`,
`diff`, `Directive`, `dist`, `DKIM`, `DLL`, `DMARC`, `DML`, `DNA`, `DNS`, `Dock`, `DocuSign`, `DOM`, `dorny`,
`DPA`, `DPIA`, `DSFA`, `DSGVO`, `DSN`, `dsn`, `DSNs`, `DSR`, `DTO`, `E`, `EAV`, `EC`, `ECB`, `Edge`,
`egress-restricted`, `Einwilligung`, `embeddings`, `Empfangsbestätigung`, `enqueue`, `Enterprise`, `Entra`,
`entrypoint`, `enum`, `Env`, `env`, `env-only`, `ERP`, `Esc`, `Escape`, `ETag`, `EU-region`, `EUR`, `European`,
`evaluator`, `Excel`, `Exchange`, `F`, `Fable`, `FAQ`, `Fastmail`, `favicons`, `FE`, `Firefox`, `FK`, `Flash`,
`Flash-Lite`, `Fonts`, `forbidigo`, `Forrester`, `fp`, `frontend`, `Frontier`, `FX`, `G`, `Garnet`, `Gartner`,
`Gatekeeper`, `GB`, `GBP`, `GC`, `gcal`, `GCM`, `Geist`, `Gemini`, `Gemma`, `Geocoding`, `geocoding`, `GH`,
`GiB`, `GIF`, `GIN`, `gitignored`, `gitleaks`, `glob`, `GmbH`, `GNU`, `go-arch-lint`, `GoBD`, `golangci-lint`,
`goroutine`, `goroutines`, `govulncheck`, `GPL`, `GPT`, `GPU`, `Graph`, `graphcal`, `Grep`, `grep`, `Groq`,
`GUC`, `H`, `Haiku`, `Handelsbrief`, `HardPass`, `HEIC`, `HEIF`, `HGB`, `HMAC`, `HMAC-SHA`, `HTML`, `HTTP`,
`http`, `https`, `HubSpot`, `IAM`, `IANA`, `ICP`, `ICS`, `ID`, `id`, `idempotency`, `Idempotency-Key`, `IDNA`,
`IDs`, `IMAPS`, `Impressum`, `Inc`, `Indigo`, `Inspector`, `Intel`, `Intelligence`, `iOS`, `IP`, `iPad`,
`iPhone`, `IR`, `ISC`, `ISO`, `JavaScript`, `Jev`, `JPEG`, `jsdom`, `JSON`, `JSON-LD`, `JSON-RPC`,
`JSON-schema`, `JSONL`, `JSONPath`, `JSX`, `JWKS`, `K`, `KB`, `kB`, `Kev`, `Keychain`, `keyvault`, `KiB`,
`Kubernetes`, `KV`, `Lars`, `Laya`, `lcov`, `Levenshtein`, `Linux`, `LiteLLM`, `LLM`, `localhost`, `lockfile`,
`Logistik`, `London`, `Lucide`, `M`, `MAC`, `Mac`, `Mach-O`, `macrotask`, `MAJOR`, `Makefile`, `margince`,
`margince-constellation`, `margince-migrate`, `Markdown`, `markdown`, `MB`, `Meet`, `Message-ID`, `MiB`,
`micro-USD`, `middleware`, `MIME`, `MinIO`, `MINOR`, `Mistral`, `Mistral's`, `MIT`, `Mitarbeiterinformation`,
`MLX`, `MRL`, `ms`, `MSVC`, `MSYS`, `mtime`, `MX`, `Mythos`, `N`, `NaN`, `NAT`, `nav`, `NetworkPolicy`, `NFC`,
`nginx`, `nginx-unprivileged`, `nil`, `NL`, `Nomad`, `Nominatim`, `non-match`, `nonce`, `Nordwind`,
`NormalizedRecord`, `notarization`, `npm`, `Nr`, `NULL`, `null`, `OA`, `oapi-codegen`, `OAuth`, `oauth`,
`oauth-consent`, `OCI`, `Office`, `OIDC`, `OpenAI`, `OpenAI-compatible`, `OpenAPI`, `OpenRouter-brokered`,
`OpenStreetMap`, `Ops`, `ops`, `Opus`, `OS`, `Outfit`, `Outlook`, `OWL`, `P`, `PATCH`, `PDF`, `PDFs`, `PERF`,
`perfbench`, `pgvector`, `pgx`, `pgxpool`, `PII`, `PIM`, `PK`, `Playwright`, `PNG`, `PoC`, `POSIX`, `POST`,
`PostgreSQL`, `POSTs`, `PowerShell`, `PR`, `Pressly`, `Pro`, `Prometheus`, `PRs`, `psql`, `PTR`, `Pub`,
`punycode`, `purl`, `purls`, `push-capable`, `px`, `PyPI`, `Python`, `QC`, `QC-only`, `Qwen`, `RAM`, `RAW`,
`re-authenticate`, `Re-certify`, `re-certify`, `Reachability`, `README`, `real-Postgres`, `Rekor`, `Renovate`,
`reset-password`, `RFC`, `River`, `RLS`, `Rosetta`, `RPC`, `RRF`, `RSS`, `runtime-DDL`, `Safari`, `SAR`,
`SBOM`, `SBOMs`, `SDR`, `SEK`, `semver`, `send-capable`, `SendGrid`, `SHA`, `sha`, `SHACL`, `Shopify`,
`Shopware`, `Shortlist`, `Shortlists`, `SIGINT`, `SIGKILL`, `SKU`, `SLA`, `slog`, `slug`, `SmartScreen`, `SMB`,
`SMTP`, `Sonnet`, `SPA`, `Sparkles`, `SPDX`, `spdx`, `SPF`, `SQL`, `SQLSTATE`, `SSE`, `SSO`, `SSRF`,
`Startseite`, `std`, `stderr`, `stdin`, `stdio`, `stdlib`, `stdout`, `storekit`, `Storybook`, `Streamable`,
`Stripe`, `struct`, `structs`, `Studio`, `Sub`, `sub-issue`, `Surface-B`, `Surfe`, `Svix`, `syft`, `SyncOnce`,
`syncToken`, `T`, `Tailwind`, `TCP`, `ThreadKey`, `Tiếng`, `TLS`, `tools-python`, `Trang`, `TS`, `tsc`, `TSX`,
`TTL`, `TXT`, `txt`, `TypeSafe`, `TypeScript`, `UA`, `UAT`, `UI`, `UID`, `und`, `UNIX`, `Unix`, `unix`, `UPN`,
`URI`, `URIs`, `URL`, `URL-safe`, `URLs`, `USB`, `USP`, `UTC`, `UUID`, `uuid`, `UWG`, `Valkey`, `var`, `vCard`,
`Ventura`, `Verarbeitungsverzeichnis`, `Vertex`, `vet`, `VIES`, `Vietnam`, `Vietnamese`, `Visual`, `Vitest`,
`vitest`, `Việt`, `Voice-DNA`, `VRAM`, `VS`, `vuln`, `Wappalyzer`, `WatchRenewer`, `WCAG`, `WebAssembly`,
`WebGL`, `WebP`, `Wettbewerbszentrale`, `worktrees`, `WSL`, `X`, `Xcode`, `Y`, `YAML`, `Zalo`, `Zürich`
