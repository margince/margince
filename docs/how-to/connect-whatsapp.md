# Connect a WhatsApp Business number

An admin connects one WhatsApp Business phone number, and Meta then posts the number's messages to this
installation. The steps below are the connection and the webhook. They stop where a received message waits to be
filed: a stored item is read back by the `whatsapp_ingest` job, and filing it as an activity is not wired yet. The
REST surface is the contract (`backend/api/crm.yaml`, `/channel-connections/whatsapp*` and `/channel-events/whatsapp`).

## What you need

- A Meta app with the WhatsApp product, and a WhatsApp Business Account that holds the number.
- The WhatsApp Business Account id, the phone number id, and the number in display form.
- A system-user access token for the account, and the app secret from the app's settings.
- A credential store on this installation, because both secrets are sealed in it.
- `--public-base-url` set, so Meta can reach `<public_base_url>/v1/channel-events/whatsapp`.
- A worker running. The API stores each item and queues its job; the worker reads it back.

## Connect the number

An admin sends `POST /v1/channel-connections/whatsapp` with the five values above. The server makes no call to
Meta. It does three things:

1. It makes a random verify token.
2. It seals the access token, and the app secret together with the verify token, in the credential store.
3. It writes one `channel_connection` row with `provider = whatsapp`, the phone number id in `account_id`, and the
   status `connected`, together with its audit row. The admin is the approver, so there is no waiting state.

The answer carries the verify token once. Nothing reads it back, and neither secret is returned or written to the
audit row. Connecting a number that already has a live connection answers `409`. Only a human admin can connect,
list or disconnect a number.

## Subscribe the webhook in Meta

In the Meta app, set the callback URL to `<public_base_url>/v1/channel-events/whatsapp` and the verify token to the
one the connection returned. Subscribe the fields `messages`, `smb_message_echoes`, `smb_app_state_sync`,
`history` and `account_update`. Meta then calls `GET` with `hub.challenge`, and the route echoes it as plain text
when the token belongs to a connected number. Any other token answers `403`.

## What the webhook does with a delivery

- It checks `X-Hub-Signature-256` against the app secret of every connected number before it decodes anything. A
  body that no secret signs answers `403` and stores nothing.
- A secret covers only its own numbers. Items for a number that the verified secrets do not cover are skipped, so
  two numbers under two Meta apps cannot file under each other.
- Each message, echo and status that passes is one `raw_capture` row (`source_system = whatsapp`) and one
  `whatsapp_ingest` job, written in one transaction. A redelivered item refreshes its row and queues no second job.
- An item from an erased customer is dropped before anything is stored. A customer with a phone is keyed by the phone digits, and one with only a business-scoped user id by that id inside the WhatsApp Business Account.
- An item that cannot be decoded is skipped and counted, and the rest of the batch is kept. Fields this version does
  not handle, such as `history`, are acknowledged and not stored.
- The call makes no request to Meta and downloads no media, so it answers inside Meta's time limit. A `503` means the
  delivery could not be stored, and Meta retries it.

## Disconnect

`DELETE /v1/channel-connections/whatsapp/{id}` archives the connection and destroys the sealed secrets. The route then
stores nothing for the number. Connecting the same number again makes a new connection and a new verify token.
