<!-- prose:plain -->
# REST API

The rules every call to the Margince REST API follows, for a script or another system that holds a
passport. The operations themselves are in the OpenAPI document; to make a passport, see
[how-to/mint-a-passport.md](../how-to/mint-a-passport.md). The contract that decides all of this is
`backend/api/crm.yaml`.

## Address and sign-in

| What | Value |
|---|---|
| Base URL | The install's API address with `/v1`. `GET /v1/passports` answers it as `api_base_url`; [configuration.md](programs.md#public-base-url) says where it comes from. |
| Credential | `Authorization: Bearer mgp_…`, a passport. The web app signs in with the `crm_session` cookie instead. |
| A bad or revoked passport | `401`. Every call checks the passport again, so a revoke takes effect on the next call. |
| An operation only a human may call | `403 permission_denied`. These operations are left out of the OpenAPI document. |

## The OpenAPI document

`GET /v1/openapi.json` answers an OpenAPI 3.1 document of every operation a passport can call, with
`servers` set to this install's base URL. A passport or a session can read it.

```sh
curl -H "Authorization: Bearer $MARGINCE_PASSPORT" https://crm.example.com/v1/openapi.json -o margince.json
```

Load the saved file in an OpenAPI viewer or a client generator. `backend/tools/gen-agentcontract` cuts
one agent contract from `crm.yaml`. The server sends it here as JSON, and as `openapi.yaml` in the
skill ZIP (`GET /v1/agent-bundle`).
[explanation/agent-surface.md](../explanation/agent-surface.md#the-margince-skill-the-contract-an-ai-tool-reads)
says what the cut keeps.

## Lists

A list takes `cursor` and `limit` as query parameters, and some lists also take `sort`; each
operation in the OpenAPI document shows which. A list answers `{ "data": [...], "page": {...} }`.

| Part | Rule |
|---|---|
| `limit` | 1 to 200, and 50 when left out, unless the operation sets its own bounds. |
| `page.next_cursor` | Pass it as `cursor` for the next page. `null` on the last page, where `page.has_more` is `false`. |
| `page.total` | How many rows match, when the list counts them. A list that leaves it out does not count. |
| `sort` | One field, with `-` in front to sort from high to low, such as `-updated_at`. The default is `-created_at,id`. |
| A changed `sort` mid-walk | `422 validation_error`, with `cursor_param_mismatch` as the code of the `cursor` entry in `details.errors`. Start again without `cursor`. |
| A changed filter mid-walk | Not checked. Start again without `cursor`, or the next pages answer the new filter. |

## Errors

An error answers `application/problem+json` (RFC 7807) with a machine `code` to branch on:

```json
{"type": "https://errors.gradion.com/version_skew", "title": "Conflict", "status": 409,
 "code": "version_skew", "detail": "This record changed after you opened it. ..."}
```

`detail` is a sentence for a human to read. `details` carries structured context when there is
some: an `errors` array of `{field, code, message}` on a `422`, or `existing_id` on a duplicate. The
statuses each operation can answer are listed with its responses in the OpenAPI document.

A path, query or header parameter whose value does not parse, or that is missing, answers
`422 validation_error`, with `invalid_parameter` as the code of its entry. Every operation that can
answer this declares the `422`: `backend/internal/compose/paramrefusaldeclared_test.go` sends each
operation a bad value for each parameter and checks.

## Safe retries and concurrent edits

| Header | Rule |
|---|---|
| `Idempotency-Key` | Once the first request has finished, a retry with the same key within 24 hours answers its status and body. The key is held per caller and path, and the same key with another body answers `409 idempotency_key_conflict`. While the first request is still running, a retry answers `409 idempotency_key_in_progress` with a `Retry-After` header; send it again after the wait until it answers with the recorded result. If the first request failed after it had started, a retry answers `409 idempotency_key_failed`: check whether it had an effect before using a new key. On an operation that runs an agent tool, the key is held per caller and tool, and the same key with other arguments answers `409 conflict`. Only an operation that lists the header in the OpenAPI document reads it. |
| `If-Match` | The record `version` you last read. If the record changed since, the write answers `409 version_skew`. Read it again and retry. |

## Changes that wait for a human

A passport's change runs at once or waits for a human, per the operation's tier
([explanation/authorization.md](../explanation/authorization.md#autonomy-tiers--how-agent-actions-are-governed---)).

1. A change that waits answers `403 approval_required`. Its `detail` text names the approval id;
   no other field carries it.
2. A human decides it in the app ([handbook/approvals.md](../handbook/approvals.md)). The passport
   can follow it with `GET /v1/approvals/{id}`.
3. Send the same request again with `X-Approval-Token: <approval id>`, within 15 minutes of the
   approval. Before the decision it answers `403 approval_required` again; do not stage a second one.

The repeated request can still be refused:

| Answer | Cause |
|---|---|
| `403 approval_token_invalid` | A different body, a rejected approval, one past its 15 minutes, or a second use. |
| `409 version_skew` | The record changed after the approval. |

## What the server sets

The server stamps `captured_by` from the passport and ignores it in a request body. `source`, which
says what produced the record (such as `api`), is yours to set; `source_system` names your system
when you import. A record is archived, not deleted: an archive operation sets `archived_at`, and the
record stays readable by id. A few operations remove a part of a record, such as
`removeOfferLineItem`.

## An id from your system

A contact, company, deal or project can keep `source_id`, its id in your system. You set it on
create, next to `source_system`. The reasons are in [outside-ids.md](../explanation/outside-ids.md).

| Create request | Answer |
|---|---|
| `source_id` with no `source_system` | `422` |
| An empty `source_id` | `422` |
| `source_system` or `source_id` over 128 characters | `422` |
| A `source_system` that starts with `mirror:`, from a caller that is not a declared importer | `422` |
| The same pair as a record of the same kind, whether or not you may read it | `409` |

Only `listCompanies` and `listDeals` can look a record up by it. Send both query parameters to find
the one record. Each also works alone. `source_system` lists every record from that system, and
`source_id` matches that id in any system. An empty value, or one over 128 characters, is also `422`
on the parameters.

## Company types from an extension

A company has core's `relationship_types`, a closed list. An installed extension can add types of its own,
as `ext:<unit>:<type>`. They come back in the separate field `extension_relationship_types`, so the closed list
never holds a value your client does not know.

| Request | Answer |
|---|---|
| `PATCH /v1/companies/{id}` with `extension_relationship_types` | Replaces the extension types of the company. An empty array clears them. Leaving the field out changes nothing. |
| A value no installed extension declares | `422` on `extension_relationship_types`, code `invalid_enum`. |
| `GET /v1/company-types` | Every declared value with its `label` and `unit`, for any signed-in user. |
| `GET /v1/companies?extension_relationship_type=ext:acme:reseller` | The companies that carry it. A value no extension declares is `422`. |
| `GET /v1/companies?relationship_type=...` | Takes core types only, as before. An extension value is `422`. |

A company keeps a value when its extension is later removed. A save that leaves it unchanged is still accepted.
The steps for the extension's side are in [how-to/add-an-extension.md](../how-to/add-an-extension.md#declare-a-company-type).

## Record kinds from extensions

`GET /v1/record-kinds` lists the kinds of record installed extensions add, each named `ext:<unit>:<kind>`. A signed-in
seat sees only the kinds it may read. Each entry carries the `display_name`, the `fields`, the `natural_key`, the
`delete_policy` and the `actions` the seat holds. The list is empty when no extension declares a kind or the seat may read none.
The steps for the extension's side are in [how-to/add-an-extension.md](../how-to/add-an-extension.md#declare-a-record-kind).

## Events out

Margince sends events to your system as signed webhooks. To register one, see
[how-to/register-a-webhook.md](../how-to/register-a-webhook.md).
