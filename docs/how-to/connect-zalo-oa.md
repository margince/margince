# Connect a Zalo Official Account

An admin connects one Zalo Official Account (OA), and Zalo then posts the account's chat events to this
installation. The account's messages file as conversation entries through a team seat, as a shared mailbox's mail
does. The REST surface is the contract (`backend/api/crm.yaml`, `/channel-connections/zalo-oa*` and
`/channel-events/zalo-oa`).

## What you need

- A Zalo app that the OA has authorized, and its app id and app secret key.
- The OA id and the OA secret key that signs webhook events.
- An access token and a refresh token for the OA, from the OAuth grant.
- A team that reads the OA's messages.
- A credential store on this installation, because every secret is sealed in it.
- `--public-base-url` set, so Zalo can reach `<public_base_url>/v1/channel-events/zalo-oa`.
- A worker running. The API stores each message and queues its job; the worker files it and renews the tokens.

## Connect the account

An admin sends `POST /v1/channel-connections/zalo-oa` with the values above, a display name and `readerTeamId`.
The server makes no call to Zalo. It does four things in one transaction:

1. It seals the access and refresh tokens together in the credential store.
2. It seals the app secret and the OA secret key together in the credential store.
3. It makes the OA's team seat, read by the reader team and starting `held`.
4. It writes one `channel_connection` row with `provider = zalo_oa`, the OA id in `account_id`, the app id in
   `channel_id` and the status `connected`, together with its audit row. The admin is the approver, so there is no
   waiting state.

No secret is returned, logged or written to the audit row. Connecting an OA that already has a live connection
answers `409`. Only a human admin can connect, list, change the visibility of or disconnect an OA.

## Who reads the messages

A held seat shows its messages to the reader team only. `PATCH /v1/channel-connections/zalo-oa/{id}/visibility` with
`mail_visibility: shared` and `confirm: true` makes the next message follow the sharing of the contact it is filed on,
as for a shared mailbox. Sharing needs the workspace setting `capture.shared_posture_allowed`. Messages filed before
the change keep their audience.

## Subscribe the webhook in Zalo

In the Zalo app, set the webhook URL to `<public_base_url>/v1/channel-events/zalo-oa` and subscribe the user and OA
message events. Zalo expects a `200` within two seconds and retries a failed delivery after 30 seconds, 5 minutes,
15 minutes, 30 minutes and one hour.

## What the webhook does with an event

- It checks `X-ZEvent-Signature` against the OA secret key of every connected OA before it reads anything but the
  `app_id` and `timestamp` the signature covers. An event that no key signs answers `403` and stores nothing.
- A key covers only its own OA: the event's OA id and app id must both be those of the connection whose key
  verified. Any other event is skipped.
- The OA id is the recipient of a `user_send_*` event and the sender of an `oa_send_*` event.
- Each message that passes is one `raw_capture` row (`source_system = zalo_oa`, keyed by OA id and `msg_id`) and one
  `zalo_oa_ingest` job, written in one transaction. A retried delivery refreshes its row and queues no second job.
- Follow, unfollow, seen and reaction events are acknowledged with `200` and stored nowhere.
- A message from an erased customer is dropped before anything is stored. The customer is keyed by the OA id and the
  customer's user id, because Zalo user ids are treated as scoped to one OA.
- The call makes no request to Zalo, so it answers inside Zalo's time limit. A `503` means the event could not be
  stored, and Zalo retries it.

## How a message is filed

The `zalo_oa_ingest` job files the message through the same entry as every chat network, while the connection is
still live. A customer's message and an OA Admin reply are one thread per customer on one contact. The thread is
capture-only: a reply from Margince into it is refused, so staff answer in Zalo. An attachment files as a placeholder
naming its type until media is downloaded. A connection disconnected between storing and filing, or a customer erased
in that time, leaves nothing behind.

## Token renewal

The access token lives 25 hours and the refresh token is single use. The `zalo_token_renew` job runs every 30 minutes
and replaces the pair of every connected OA that has under six hours left. The pasted access token counts as due, so
the first run replaces it with a pair whose expiry Zalo stated.

- The new pair is sealed before the row names it, and the old pair is destroyed after the row does.
- A run holds the connection's row lock from the Zalo call until the new pair is committed. A second run skips a
  locked row, so two runs never spend one refresh token.
- If sealing fails after Zalo issued the new pair, the new pair is lost and the old refresh token no longer works.
  The next run then sees Zalo refuse it, and the connection becomes `reauth_required`.
- When Zalo refuses the refresh token, the connection becomes `reauth_required`. Disconnect it and connect it again
  with new tokens. A `reauth_required` connection still receives and files messages, because their signature does
  not use the access token. Only sending and calls that need the token fail.

## Disconnect

`DELETE /v1/channel-connections/zalo-oa/{id}` archives the connection, ends its seat and destroys the sealed secrets.
The route then stores nothing for the OA, and what it filed stays. Connecting the same OA again makes a new connection.

## Known limit

One Zalo user id can be bound to a contact under only one OA, until the unscoped identity index on
`contact_channel_identity` is dropped. A second OA that sees the same user id stores and files the message, but cannot
bind that id to a second contact.
