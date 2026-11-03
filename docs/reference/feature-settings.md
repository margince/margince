<!-- prose:plain -->
# Feature settings

How some features read their settings: AI payload capture, capture and lists, the test mailbox,
uploads, `security.txt`, rates and sales reporting. Which settings turn on which feature is in
[What each feature needs](what-each-feature-needs.md).

## AI payload capture and its window (api, worker)

`ai.capture_payloads` is off by default. To turn it on stores the whole request and response of the
model in `ai_call_payload`. For a reading of a meeting transcript, that request *is* the transcript:
the most complete copy of the words of someone that this product holds.

The `ai_call_payload` / `content` row in `retention_policy` limits that. Bootstrap sets it to 365
days and `enabled`, so the retention engine erases what is older from the first sweep. The number is
a **default an admin can edit**, and each installation decides its own. Three things decide it:

- **What it limits.** How long a captured transcript, contract or draft stays on disk after the
  work is done.
- **What it is for.** To debug a call and to audit what was sent are questions of days to weeks. A
  year of them is a year of the words of someone, kept for a lane nobody reads.
- **What it does not limit.** An Art. 17 erasure reaches these payloads through the record a call
  named, and by matching the addresses of the subject in the text. A call that names no record, and
  whose text has no address, is reached by none of the two. For those calls this window is the one
  end that always holds, and a shorter window makes it come sooner. The two lanes, and why the
  record link helps but does not set the limit, are in
  [privacy-and-consent.md](../explanation/privacy-and-consent.md).

## Capture, company context and lists

`capture.trace_payloads` (default `true`) keeps the sender of each traced message and a limited
subject (320 and 300 characters, never a body). It keeps them in the 24-hour Capture activity trace
every member sees under Settings. It covers messages dropped because every party was inside your
own domains. The CRM stores nothing else about those, and they are what an operator looks for when
a message is missing.

- It is on by default, because the trace exists to answer why a message did not come in. A page of
  decisions that names nobody cannot answer that.
- A member reads only rows from their own connections, and no grant gives more. So this shows the
  own mail of a member back to them.
- Set it to `false` where a works agreement needs that; the trace then keeps recording every
  decision, and names nobody.
- Only this file can set it: there is no API and no switch in the app, so no member can change it
  for colleagues.
- The sweep each hour deletes payloads with the rows that carry them. Margince never writes the
  address of an erased subject, no matter what the posture says. An Art. 17 request inside the
  window reaches what is already there.

`capture.skip_reserved_domain_proposals` (default `true`) keeps a sender on an RFC 2606 reserved name
out of the contact review queue. That is `example.com`, `example.net` and `example.org` with the
names under them, and anything under `.test`, `.example`, `.invalid` or `.localhost`. Their mail is
still captured and stays on the timeline.

Only the `capture_counterparty` proposal ("is this a contact to keep?") is not raised. So no
approval and no notice show up for it. The review sweep closes the open question of such a sender as
`rejected`, and does not ask it. Proposals raised before the setting applied stay open; refuse
them in the decision queue. Set it to `false` only for an installation that runs a test mailbox and
needs those senders as proposals.

`company_context.rollout` is the server company context feature, in order:

- `off` turns off context reads, injection, and the new onboarding surface;
- `read` turns on the shared read model and Company Context settings;
- `tasks` also adds limited context into declared AI tasks;
- `onboarding` also turns on the first-run onboarding steps.

The default is `onboarding`. To move back is an off switch you can undo, and it never deletes
confirmed company data.

`lists.enabled` turns Live Lists and Shortlists on or off. The default is `true`. Set it to `false`
to hide them:

- the `/v1/lists` routes answer 404;
- the `list_id` filter of the contact, company, deal and lead lists answers 404, and the filtered
  export refuses a `list_id` source;
- the company page names no list, and no agent list tool is registered;
- `/me` reports `settings_availability.lists: false`, so no screen offers them;
- the 15-minute Live List check of the worker records nothing. So no list history grows, and no
  `list.evaluated` event goes out.

The worker reads the same file, so set it in the file both roles load. To turn it off hides lists
without deleting any. To turn it back on shows them as they were. The first check after that
records who joined each list since the last check, and who is no longer on it.

## `POST /v1/connectors/test_mailbox/connect`: the QC-only fake mailbox

`operations.allow_test_mailbox` in `margince.yaml` gates it, with a compiled default of **false in
every posture, dev included**. `config/margince.dev.yaml` arms it for a dev stack, the same as
`allow_data_reset`. An installation that did not arm it gets the same `connector_unsupported` 422
that a real provider with no setup returns. The connect endpoint, the connector registration and its
right to send all turn on this one flag.

```yaml
operations:
  allow_test_mailbox: true   # dev/test only; omit or false everywhere else
```

The `test_mailbox` connector does both capture and send, with no real network. `SendEmail` refuses
any address outside the RFC 2606 reserved domains (`example.com`/`.net`/`.org`,
`.test`/`.example`/`.invalid`/`.localhost`), and never calls out. Its own `Sync` sends back what it
sent. It matches that against the outbound activity by RFC822 Message-ID, and does not make a copy.

The UI never offers it (it is not in `MAIL_PROVIDERS`). The connect endpoint is the only way to
create such a connection. That lets a QC test create and remove one within a test run, without
starting a process again.

## Uploads

The `uploads:` block sets the request size each route that carries a **file** may read. Every other
route stays on the 1 MiB JSON limit. That limit is a security rule, and you cannot change it. Some
handlers read the body with no limit of their own, and two of those routes need no sign-in.

One JSON route reads more. `POST /mcp` with `Content-Type: application/json` takes up to 8 MiB
(`agents.MaxMCPRequestBytes`), because `attach_document` carries a file in the call as base64. That
leaves room for a file of about 6.2 MB. `attach_document` takes the smaller of that and
`uploads.attachment_mb`. You cannot change the 8 MiB or the 6.2 MB limit. A request over 8 MiB gets
`413`, with the limit named.

A passport's `POST /v1/attachments` takes the same file limit as `attach_document`. A file over that
limit answers `422` on `file`. The agent gate reads the whole multipart body first, up to the tool's
8 MiB, and a larger body answers `422` on `body` with code `too_large`. A human's upload keeps
`uploads.attachment_mb`.

Only one tool may use the 8 MiB body. Every other tool refuses input over 1 MiB before it runs,
because `ToolSpec.MaxArgsBytes` starts at the JSON limit and only `attach_document` raises it.

One process holds at most 4 MCP requests over 1 MiB at once (`maxLargeMCPBodiesInFlight` in
`backend/internal/modules/agents/httpmcp.go`). Each one sits in memory many times while it is
read. Each agent may hold only one of them, and its second gets `429`. When all 4 are in use, the next gets `503` with
`Retry-After: 1`. When it states a `Content-Length`, it gets that answer before its body is read.

Once a request holds a place, the rest of its body must come within
10 seconds (`largeBodyReadDeadline`). If it does not, the answer is `408`, and the place is free
again.

Every upload that adds a document, from the app or from `attach_document`, must be one of the kinds
in `attachmentTypes` (`backend/internal/modules/activities/attachmenttypes.go`). In the app, any
other kind gets `422 unsupported_file_type`. Over MCP, `attach_document` answers with a tool error
(`isError: true`) that names the same code, and the HTTP status stays `200`. HTML and archive files are accepted, because Margince only hands a
stored file back as a download. `.svg` files and programs are refused.

The declared type must be in the table. When the file name ends in a type from the table too, the
file is stored under that type. Windows, for one, declares a `.csv` file as
`application/vnd.ms-excel`.

In every other case the declared type is kept, and the name does not
matter. An empty or `application/octet-stream` type, which browsers send for `.msg` and `.md`, is
read from the file name. Margince does not look at the bytes. Files that come in with an email are
stored in any kind, as a record of what was sent.

**Decimal megabytes**: `25` means 25,000,000 bytes, not 26,214,400. The value here is the number
the 413 of the server names, and the number the upload form states. The unit is decimal so those
three agree. A binary constant reads as "25 MB" in a sentence, but lets in 4.8% more.

The limit covers the **whole request**, part framing included. The framing adds less than 1,000
bytes, so it only matters close to the limit. A client that refuses before it sends should leave
itself that much room.

A key you do not set takes the default. A value outside **1–100 MB** is a boot error that names the
key, and that includes `0`, which would refuse every upload. Margince never moves such a value into
range. Past 100 MB, upload right to the object store, and do not raise the number. The request goes
through a temp file system of the api on its way to the store.

`attachment_mb` is also published, read-only, as `max_upload_bytes` on
`GET /v1/installation/settings`, which every role may read. An upload surface uses it to state and
hold the limit of this installation, not one compiled into the client. So a file over the limit is
refused before it is sent. A change applies when the process starts again, like every other key in
this file.

The block does not decide which routes may carry a file at all. That list is declared in source
(`internal/compose/bodyceiling.go`). It stops a route that carries no file from getting the file
limit by sending a multipart header. To add to it is a code change, with two fitness gates over it.

## security.txt

The `web.security_txt` block publishes an [RFC 9116](https://www.rfc-editor.org/rfc/rfc9116) file at
`/.well-known/security.txt`. That is the place a security researcher looks to learn who to tell
about a security hole. The contact is the **operator**: the one who runs this installation, not the
one who writes the software. Nothing is compiled in, so an installation without the block answers
that path with 404.

```yaml
web:
  security_txt:
    contact:                      # required, one or more
      - mailto:security@example.org
      - https://example.org/report-a-vulnerability
    expires: "2030-01-01T00:00:00Z"   # required, RFC 3339
    policy: https://example.org/disclosure-policy   # optional
    preferred_languages: [en, de]                   # optional
```

| Key | Rule | Shows as |
|---|---|---|
| `contact` | at least one `mailto:`, `https:` or `tel:` URI | one `Contact:` line each, in order |
| `expires` | required; an RFC 3339 date and time | `Expires:`, in UTC |
| `policy` | not required; an `https:` URL | `Policy:` |
| `preferred_languages` | not required; language tags such as `en` or `de-CH` | one `Preferred-Languages:` line that holds them all |

A value that breaks a rule is a boot error that names the key. An `expires` that has already passed,
or that ends more than a year from now (RFC 9116 asks for less), is a boot **warning**. The api keeps
serving the file, because RFC 9116 leaves it to the reader to judge when a file is too old. But the
log says to move the date.

The api serves the file, since it holds this config, as `text/plain; charset=utf-8`. Route
`/.well-known/security.txt` to the api by that path (see
[Route one host to two services](../how-to/deploy-margince.md#route-one-host-to-two-services)). If the ingress leaves it on the web service instead, the web tier
answers 404, the same as an installation with no file.

## Rates

The `rates:` block sets up the admin **Refresh from sources** job for the currency sheet (worker
role). A refresh never writes a rate itself. It stages **confirm-first proposals** in the approvals
inbox, and a human approves each one before it applies. Only the worker reads the block: the `api`
puts the job in the queue, and the worker gets the rates and stages them.

Model prices are not set here. Margince updates them daily from public catalogs (Settings → AI →
**Model prices**):

- `anthropic`, `openai`, `gemini` and `gemini_vertex` get their prices from models.dev, when their
  key works;
- the models hosted on OpenRouter that the installation binds get their prices from the OpenRouter
  list. So do the `openai_compatible` rows on the sheet, while something is bound there;
- every other provider keeps the prices set by hand.

Margince adds a chat or embedding model a key lists when models.dev prices it in the same lane. It
never writes over a price set by hand. To turn **Auto-sync daily** off stops the daily job;
**Refresh model prices** still runs it when asked.

| field | default | what it does |
|---|---|---|
| `fx_source` | `https://api.frankfurter.dev/v1/latest` | An FX JSON API that answers rates against a base (`{base,rates}`, asked with `?base=&symbols=`). The default is the free ECB source, which needs no key. |
| `fx_currencies` | `[USD, GBP, CHF]` | Other currencies the FX refresh makes proposals for, to **fill an empty rate sheet the first time**. A new install tracks none, so the refresh would have nothing to get. Once the sheet has rows, the refresh prices the tracked currencies again, and this set is not used. Each entry must have the **ISO 4217 shape** (three letters, such as `EUR`) and show up once, or boot fails: the same shape check as `base_currency`. Margince does not check that the code exists. A code with the right shape that the source does not support (`USX`) is read, and then the source skips it with a warning in the log. |

The **FX refresh** needs a bound `rate_extract` model, in the stored binding of the installation.
Without one, it does nothing. `fx_source` and `fx_currencies` both have defaults, so it always has
something to do, even with no `rates:` block. It never applies a rate on its own. It makes a
proposal of a rate from the live source, and applies it only when a human approves. A deal in a
currency other than EUR with no approved rate still fails closed (never a `rate=1` nobody sees).

Margince still reads a `model_pricing:` key that an older file still has, does not use it, and
writes a warning at boot; remove it.

## Sales reporting

Reporting is always available in the API, web UI, MCP tools and workers. Margince accepts the
`analytics.performance_enabled` key but does not use it, so older config files still load. Remove it
from operator files. [The reporting how-to](../how-to/operate-reporting.md) covers setup, how to stop a
schedule for good, and rollback.
