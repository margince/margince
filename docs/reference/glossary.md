# Glossary

Each technical name a plain page may use, with what it means. A plain page uses general words from its
area's word list and the names below. A name matches only as written here.

To add a name, add a row with a meaning of at least three words. An ordinary English word does not belong
here: put it in the area's word list instead. A name that no plain page uses must leave this table.

| Term | Meaning |
|---|---|
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
`AA`, `AAAA`, `Act`, `AD`, `AirDrop`, `allowlist`, `Anthropic`, `Apple`, `arch-lint`, `args`, `Art`, `ASCII`,
`AST`, `Aurora`, `Austria`, `Austrian`, `Authenticode`, `axe`, `Azure`, `BDSG`, `Betriebsrat`,
`Betriebsvereinbarung`, `BetrVG`, `BI`, `BotFather`, `BYOK`, `C`, `CA`, `Cc`, `CGNAT`, `ChatGPT`, `CHF`,
`Chrome`, `chunker`, `Claude`, `claude`, `CLI`, `Cloud`, `CNAME`, `CODEOWNERS`, `CodeRabbit`, `Codex`, `codex`,
`config`, `cron`, `CRUD`, `CSRF`, `CSS`, `Ctrl`, `Ctrl-C`, `curl`, `CV`, `DAG`, `DCR`, `DDL`, `dedupe`,
`DeepSeek`, `DELETE`, `Desktop`, `dist`, `DKIM`, `DMARC`, `DNS`, `DocuSign`, `DPA`, `DPIA`, `DSFA`, `DSGVO`,
`DSN`, `DSR`, `Edge`, `Einwilligung`, `Empfangsbestätigung`, `Enterprise`, `Entra`, `enum`, `ERP`, `Esc`,
`Escape`, `EUR`, `European`, `evaluator`, `Excel`, `FAQ`, `Fastmail`, `Firefox`, `Flash-Lite`, `Fonts`,
`Forrester`, `frontend`, `FX`, `Gartner`, `Gatekeeper`, `GB`, `GBP`, `GCM`, `Geist`, `Gemini`, `Gemma`,
`Geocoding`, `gitignored`, `GNU`, `GPL`, `GPT`, `GPU`, `Graph`, `Groq`, `Haiku`, `Handelsbrief`, `HardPass`,
`HGB`, `HMAC`, `HMAC-SHA`, `HTML`, `HTTP`, `IAM`, `IANA`, `ID`, `IMAPS`, `Impressum`, `Inspector`, `Intel`,
`IP`, `JSON`, `JSONL`, `JSONPath`, `K`, `KB`, `keyvault`, `Lars`, `LLM`, `London`, `Mac`, `Makefile`,
`Markdown`, `MB`, `Mistral`, `Mitarbeiterinformation`, `MRL`, `ms`, `MSVC`, `MX`, `NAT`, `notarization`, `Nr`,
`NULL`, `OAuth`, `Office`, `OpenAI`, `OpenAI-compatible`, `Ops`, `OS`, `Outfit`, `Outlook`, `PATCH`, `PDF`,
`pgvector`, `PIM`, `POST`, `PostgreSQL`, `POSTs`, `PowerShell`, `PR`, `PTR`, `px`, `Python`, `re-authenticate`,
`Re-certify`, `re-certify`, `README`, `RFC`, `River`, `Rosetta`, `Safari`, `SAR`, `SBOMs`, `SDR`, `semver`,
`Shopify`, `Shopware`, `SKU`, `SLA`, `slug`, `SmartScreen`, `SMTP`, `Sparkles`, `SPDX`, `SPF`, `SQL`, `SSRF`,
`stderr`, `stdin`, `stdout`, `Storybook`, `Stripe`, `struct`, `Studio`, `Surfe`, `Svix`, `TCP`, `TLS`, `TTL`,
`TXT`, `TypeSafe`, `TypeScript`, `UI`, `UID`, `und`, `unix`, `URI`, `URIs`, `URL`, `URL-safe`, `USB`, `UTC`,
`uuid`, `Valkey`, `vCard`, `Ventura`, `Verarbeitungsverzeichnis`, `Vertex`, `vet`, `VIES`, `Visual`, `VS`,
`Wappalyzer`, `WCAG`, `Xcode`, `YAML`, `Zürich`
