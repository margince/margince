<!-- prose:plain -->
# Register an outbound webhook

Register an HTTPS endpoint, so that it gets signed deliveries of published domain events, which are tried again when they fail.
Then check the signature, look at deliveries, and replay a parked one. To learn how it all works, read
[explanation/outbound-webhooks.md](../explanation/outbound-webhooks.md) first. It covers the settings
part and the delivery engine, how secrets are sealed, and the payload pipeline that starts from the contract.
It also covers versions, the states of a retry and a dead letter, and the gate that limits delivery to what the owner may see.

All of the steps below also work from the app. **Settings → Integrations** does the same create, pause,
resume, new target, archive, rotate and replay actions that this guide does with `curl`. It also has a panel for
deliveries and dead letters. Both use the same API.

**First party, outbound only.** This registers a *subscription* that Margince delivers to. It is not
an inbound receiver, and it does not install an app from a third party. Deliveries are signed HTTP POSTs of a **typed
payload that the contract generates** (`backend/api/public-events.yaml` → `internal/contracts`, one
`PublicEvent<Event>` schema for each event type). They follow the [Standard Webhooks](https://www.standardwebhooks.com/)
scheme, which Anthropic, OpenAI, Stripe and Svix use. Any ready-made library that checks Standard Webhooks
works with no change.

> **One company for each installation.** One installation serves one company. The
> server finds its one company by itself, so no request chooses a tenant, and there is no
> `X-Workspace-Slug` header. The `curl` calls below carry only the session cookie. ("Workspace" still names
> the tenant identity in the code that `WithWorkspaceTx` uses for the transaction.)

## Before you start

- **Admin or ops RBAC.** Subscriptions are integration settings for the whole company (like
  custom fields), so only `admin` and `ops` may change them. Every role may *read* a subscription and its
  deliveries.
- **A deployment signing key must be set**: `MARGINCE_WEBHOOK_KEY` (see step 1). Without it, the
  read paths still list, but create, rotate and replay answer `503 webhooks_not_configured`, and no
  delivery runs.
- **An HTTPS endpoint you control** that can check an HMAC and return `2xx`. Create refuses
  plain `http://` targets.

## 1. Set the deployment signing key

The key seals the signing secret of every subscription at rest (AES-256-GCM). It is a **base64
32-byte** key, and the API and the delivery worker share it. Mint one:

```sh
openssl rand -base64 32
```

Set it on both the API and the worker before they start (`--webhook-key` or `MARGINCE_WEBHOOK_KEY`). A
key of the wrong length is a **start error**, and the server never fills it out to the right length. When you rotate this deployment key,
nothing is sealed again on its own, so keep the key for a long time. The secret you rotate in daily work is the
one for each subscription (step 6).

```sh
export MARGINCE_WEBHOOK_KEY="$(openssl rand -base64 32)"   # then (re)start the api + worker
```

In `make dev`, the API delivers from the bus itself (`--inline-relay`). When the worker runs as its own program, it runs the same
`cg:webhooks` consumer. The same key must be set on the program that delivers.
Only the worker tries a failed delivery again. That is a River job that runs on a schedule, and the API
runs no jobs. So an installation with no worker never tries a parked delivery again.

## 2. Create a subscription

```sh
curl -X POST http://localhost:8080/v1/webhook-subscriptions \
  --cookie 'crm_session=<admin or ops session>' \
  -H 'Content-Type: application/json' \
  -d '{
        "target_url": "https://example.test/hooks/margince",
        "event_types": ["deal.stage_changed", "contact.created"]
      }'
```

- `target_url` **must be `https://`**.
- `event_types` is a **part of the published catalog, and not empty**. An unknown type is a `422`.

  The capture pipeline events (`capture.*`) have no record behind them, so the server refuses them: they name no subject to
  limit delivery by. The catalog in the code (`events.Types()` without the pipeline class) checks each
  create and update. The `SubscribableEventType` enum (`public-events.yaml`) is its copy in
  the contract. For the set in the contract, run:
  ```sh
  grep -A200 'SubscribableEventType:' backend/api/public-events.yaml | grep '^\s*- ' | sed 's/^\s*- //'
  ```

  Every type in that enum has a published `PublicEvent<Event>` schema in the same file. It shows
  the `data` shape that your receiver will see for it. Some types are in the catalog, but the server does not send or
  deliver them ([outbound-webhooks.md](../explanation/outbound-webhooks.md#5-the-fan-out-and-the-owner-scope-gate)).
  The schema of each such type says so.

The `201` answer is the subscription **and the signing secret**. It is the only time the plain secret
shows up:

```json
{
  "subscription": { "id": "…", "target_url": "https://…", "event_types": ["…"],
                    "state": "active", "version": 1, "owner_id": "…" },
  "signing_secret": "whsec_…"
}
```

**Store `signing_secret` now**: no read shows it again. If you no longer have it, rotate it (step 6);
you cannot get it back. The `owner_id` is the human who made the call, and the server sets it. Delivery only
sends events that this owner may see.

> **Base64 secrets in the URL-safe form cannot sign.** Standard Webhooks needs *standard* base64. A secret
> minted in URL-safe base64, from before the Standard Webhooks scheme, cannot be read back.
> So every matching delivery fails to sign, and becomes a dead letter. Rotate it (step 6), or create the subscription again. Both
> mint a new `whsec_…` secret in standard base64.

## 3. Check the signature on your receiver

Every delivery is a POST that carries three [Standard Webhooks](https://www.standardwebhooks.com/) headers.
Check `webhook-signature` against `{webhook-id}.{webhook-timestamp}.{raw request body}` with the
secret from step 2. Run dedupe on `webhook-id`, because the bus can deliver an event more than one time.
The delivery id stays the same in all retries.

Each try mints a new `webhook-timestamp`. So a receiver that only accepts a time within a short window
refuses a captured signature once that window passes. Inside the window, the dedupe on `webhook-id`
catches it:

| Header | Meaning |
|---|---|
| `X-Margince-Event` | only there to help: the event type, such as `deal.stage_changed` |
| `webhook-id` | the delivery id: your dedupe key, the same in all retries |
| `webhook-timestamp` | unix seconds at which the server signed this try, new on every try |
| `webhook-signature` | `v1,` + base64 HMAC-SHA256 of `{webhook-id}.{webhook-timestamp}.{body}`, keyed by the decoded bytes of the secret |

Here is a small receiver check in Go. It takes `whsec_` off the secret and **decodes the base64 to bytes**
before it uses the bytes as the HMAC key, as Standard Webhooks needs:

```go
func verify(secret, webhookID, webhookTimestamp string, body []byte, sigHeader string) bool {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(webhookID + "." + webhookTimestamp + "."))
	mac.Write(body)
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(sigHeader))
}
```

Return any `2xx` to confirm you have it. Any other code, or a timeout, is a failure, and the server tries again.
One try can take at most `10s`.

## 4. Look at deliveries

The server logs every try. List the deliveries of a subscription, newest first. This is where you look at
dead letters:

```sh
curl --cookie 'crm_session=<session>' \
  "http://localhost:8080/v1/webhook-subscriptions/<id>/deliveries?limit=50" \
  | jq '.data[] | {event_type, status, attempts, last_status_code, last_error, next_retry_at}'
```

The `status` values are the states:

- `pending`: new in line, and the first try comes soon.
- `delivered`: the receiver returned a `2xx`.
- `retrying`: failed, and parked with a `next_retry_at`. The sweeper tries again (backoff `1,2,4,8,16s`).
- `dead_lettered`: the budget of 6 tries is used up. It will not try again on its own.

`page.has_more` is true while older deliveries are past the limit.

## 5. Replay a dead-lettered delivery

Once you have fixed the receiver, replay a parked delivery. This is a **human action, with an audit record**. It sets
the try budget back, and sends the *stored* body again. So it works even after the source event is no longer on
the bus:

```sh
curl -X POST --cookie 'crm_session=<admin or ops session>' \
  http://localhost:8080/v1/webhook-subscriptions/<id>/deliveries/<deliveryId>/replay \
  | jq '{status, attempts, last_status_code}'
```

Replay sends the stored body of the delivery again **word for word**, at the `version` of its first
send. It never builds the payload again against a schema that now has new fields (an additive change).
So an old delivery replays the same as the first time.

## 6. Rotate the signing secret

The rotate call mints a new secret and returns it one time. The old secret **stops working at once**, so
put the new one into your receiver before the switch, or at the same time:

```sh
curl -X POST --cookie 'crm_session=<admin or ops session>' \
  http://localhost:8080/v1/webhook-subscriptions/<id>/rotate-secret \
  | jq -r '.signing_secret'
```

## 7. Pause, change the target, or archive

Pause, resume and a new target run under a version check. Read the current `version` and
pass it as `If-Match`:

```sh
# pause delivery without archiving
curl -X PATCH --cookie 'crm_session=<admin or ops session>' \
  -H 'If-Match: "1"' -H 'Content-Type: application/json' \
  -d '{"state": "paused"}' \
  http://localhost:8080/v1/webhook-subscriptions/<id>

# archive — stops all delivery permanently
curl -X DELETE --cookie 'crm_session=<admin or ops session>' \
  http://localhost:8080/v1/webhook-subscriptions/<id>
```

A paused subscription holds its retries until it resumes. An archived one stops delivery, and a read by id
answers `404` from then on. The list shows it only when you ask with `include_archived`.
An empty PATCH (one that sets no `state` and no `event_types`) is a `422`.

## Check it from end to end

1. **Check that the secret shows up one time.** A `GET /webhook-subscriptions/<id>` never returns
   `signing_secret`. Confirm that no read has it.
2. **Check that a signed delivery passes.** Start an event you subscribe to.

   You can change the stage of a deal for `deal.stage_changed`. Confirm that your receiver gets a POST,
   and that the HMAC matches the body as sent.
3. **Check that the owner gate holds.** Register a subscription as a rep who can see only their own deals.

   Then start `deal.stage_changed` on a deal they cannot see. Confirm that **no** delivery goes in line
   for that subscription. It never goes past what the owner may read in the app.
4. **Check retry and dead letter.** Point a subscription at an endpoint that returns `500`, and start an event.

   Watch the delivery go `retrying → … → dead_lettered` over its backoff schedule. Then fix the
   endpoint, and `replay` it to `delivered`.
5. **Check that the key gate holds.** Remove `MARGINCE_WEBHOOK_KEY` and start again.

   Reads still list, but create, rotate and replay answer `503 webhooks_not_configured`.
   No delivery goes out without a signature.
6. **Check the `data` against the schema.** Use any event you subscribe to.

   Compare the `data` field your receiver gets with the `PublicEvent<Event>` schema of that event in
   `backend/api/public-events.yaml`. Each field must match. The build step that holds this is in
   [outbound-webhooks.md](../explanation/outbound-webhooks.md#3-the-contract-first-payload-pipeline).
7. **Check a subject that has no delivery.** Subscribe to `retention.applied`.

   Then start a retention sweep that deletes `ai_call` data. Confirm that you get nothing for it. This is a
   limit we know about ([outbound-webhooks.md](../explanation/outbound-webhooks.md#5-the-fan-out-and-the-owner-scope-gate)),
   and not a bug in your setup.
8. **Check that the app matches the API.** All of the above also works from Settings → Integrations.

   The create form shows the secret one time. The deliveries panel shows the same retry, dead letter and replay
   steps that you take with `curl`.
