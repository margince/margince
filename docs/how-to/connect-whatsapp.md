# Connect a WhatsApp Business number

An admin connects one WhatsApp Business phone number, and Meta then posts the number's messages to this
installation. Each customer message, and each message the business types in the WhatsApp Business app (a
Coexistence echo), files in the customer's history through a team seat, as a shared mailbox's mail does. The
handbook page [connecting-whatsapp-and-zalo.md](../handbook/connecting-whatsapp-and-zalo.md) covers the screen. This
page is for the operator. The REST surface is the contract (`backend/api/crm.yaml`, `/channel-connections/whatsapp*`
and `/channel-events/whatsapp`).

## What you need

- A Meta app with the WhatsApp product, and a WhatsApp Business Account that holds the number.
- The WhatsApp Business Account id, the phone number id, and the number in display form.
- A system-user access token for the account, and the app secret from the app's settings. The token
  needs the `whatsapp_business_messaging` permission as well as the one that reads messages. Without it the number
  still receives, but it cannot ask a member who wrote the messages they forwarded (see below).
- A team that reads the number's messages.
- A credential store on this installation, because both secrets are sealed in it.
- `--public-base-url` set, so Meta can reach `<public_base_url>/v1/channel-events/whatsapp`.
- A worker running. The API stores each item and queues its job; the worker files it.

## Connect the number

An admin sends `POST /v1/channel-connections/whatsapp` with the values above and `readerTeamId`. The server makes no
call to Meta. It does four things in one transaction:

1. It makes a random verify token.
2. It seals the access token, and the app secret together with the verify token, in the credential store.
3. It makes the number's team seat, read by the reader team and starting `held`.
4. It writes one `channel_connection` row with `provider = whatsapp`, the phone number id in `account_id`, and the
   status `connected`, together with its audit row. The admin is the approver, so there is no waiting state.

The answer carries the verify token once. Nothing reads it back, and neither secret is returned or written to the
audit row. Connecting a number that already has a live connection answers `409`. Only a human admin can connect,
list, change the visibility of or disconnect a number.

A number that the Business app also uses (Coexistence) is onboarded through Meta's Embedded Signup, in version 4 or
5. Margince does not run Embedded Signup. Whichever way the number was onboarded, the admin pastes the same values:
the account id, the phone number id and a system-user token. Neither version has been tried against this
installation (see the end of this page).

## Who reads the messages

A held seat shows its messages to the reader team only. `PATCH /v1/channel-connections/whatsapp/{id}/visibility`
with `mail_visibility: shared` and `confirm: true` makes the next message follow the sharing of the contact it is
filed on, as for a shared mailbox. Sharing needs the workspace setting `capture.shared_posture_allowed`, and
`classified` is refused. Messages filed before the change keep their audience. The shared-mailbox routes answer `404`
for this seat, so the visibility route is the only way to change it.

## Subscribe the webhook in Meta

In the Meta app, set the callback URL to `<public_base_url>/v1/channel-events/whatsapp` and the verify token to the
one the connection returned. Meta then calls `GET` with `hub.challenge`, and the route echoes it as plain text when
the token belongs to a connected number. Any other token answers `403`.

Subscribe these webhook fields:

- `messages` carries customer messages, edits, revokes, reactions and delivery statuses. Only messages, edits and
  revokes are stored.
- `smb_message_echoes` carries what the business types in the WhatsApp Business app on a Coexistence number.

Other fields, such as `history`, `smb_app_state_sync` and `account_update`, are acknowledged and stored nowhere.
Subscribing to them changes nothing.

## What the webhook does with a delivery

- It checks `X-Hub-Signature-256` against the app secret of every connected number before it decodes anything. A
  body that no secret signs answers `403` and stores nothing.
- A secret covers only its own numbers. Items for a number that the verified secrets do not cover are skipped, so
  two numbers under two Meta apps cannot file under each other.
- Each message, echo, edit and revoke that passes is one `raw_capture` row (`source_system = whatsapp`) and one
  `whatsapp_ingest` job, written in one transaction. A redelivered item refreshes its row and queues no second job.
- A delivery status and a reaction are acknowledged and store nothing.
- An item from an erased customer is dropped before anything is stored. A customer with a phone is keyed by the phone
  digits, and one with only a business-scoped user id by that id inside the WhatsApp Business Account.
- An item that cannot be decoded is skipped and counted, and the rest of the batch is kept.
- The call makes no request to Meta, so it answers inside Meta's time limit. A `503` means the delivery could not be
  stored, and Meta retries it.

## How a message is filed

The `whatsapp_ingest` job files the item through the same entry as every chat network, while the connection is still
live: `capture.FileChatMessage` for a message, and `capture.ChangeChatMessage` for an edit or a revoke. The gate
`TestChatNetworksShareOneReceiveFileChangeAndSendPath` (`backend/gates/chatonepath_test.go`) fails when another path
builds a chat record, writes its activity row or calls a provider's send API.

- A customer message and a business echo are one thread per customer on one contact, in either order of arrival. The
  thread key names the phone number id, so two numbers never share a thread, even when the customer and the message
  id repeat.
- The audience follows the contact as for mail. A held seat keeps a message from colleagues outside the reader team
  until the contact is shared or an admin shares the seat.
- The thread is capture-only. A reply from Margince into it answers `422` with `conversation_capture_only`, so the
  team answers in WhatsApp. The staging check of an agent refuses it the same way.
- An edit changes the text of the filed message, and an older edit changes nothing. A revoke removes nothing on a
  company number, because any member of the team may delete there. A revoke that arrives before its message is noted,
  so a copy that arrives after it is not filed.
- The worker keeps the files of a message. It downloads them from Meta over https, from allowed hosts only, and it
  checks each redirect. The limits are 25 MiB for a file, 20 files and 50 MiB for a message. A file that is too
  large, is refused, fails its checksum, or stays unavailable after retries is not kept. The message is still filed
  with a note that the file was not kept, as for mail. A caption stays the message text.
- A number disconnected between storing and filing, or a customer erased in that time, leaves nothing behind. The
  stored item is destroyed.
- Erasing a contact destroys the raw originals of its messages. An erased phone or user id is not stored again, and
  an erased business-scoped id is erased only under its own account.
- A job that runs twice, or at the same moment in two workers, files the message once. A job that fails after part of
  its work leaves nothing, and its retry files the message once.

## Disconnect

`DELETE /v1/channel-connections/whatsapp/{id}` archives the connection, ends its seat and destroys the sealed secrets.
The route then stores nothing for the number, and what it filed stays. Connecting the same number again makes a new
connection and a new verify token, and the thread keys stay the same. Use the same reader team, or the answer is
`409`.

## What the tests prove, and what needs a live account

The tests use the real routes, the real signature check, the real job worker and the real filing path. Only Meta is
simulated, with the delivery shapes the Graph API documents. They cover these cases:

- connect, list, visibility change and disconnect, with the secrets staying sealed;
- the verify challenge, a bad signature, a secret that covers only its own numbers, and an unknown number;
- one thread on one contact for a customer message and an echo, in either order;
- a held seat, and sharing opening the next message;
- edits, revokes, reactions and statuses;
- an erased phone or user id, and the erasure of a contact destroying its originals;
- the same message id on two numbers, a reconnect that keeps its thread, and a disconnect between storing and
  filing;
- a job that runs twice or at once, and a job that fails after part of its work.

These need a live number and a Meta app before you trust them:

- That Meta posts `smb_message_echoes` for a Coexistence number, and what it sends for an edit made in the Business
  app.
- The real fields of an edit and a revoke, and whether an edit arrives with the new text.
- Onboarding through Embedded Signup version 4 or 5, and the token and ids it gives.
- Business-scoped user ids, and what Meta sends when a customer hides a phone number.
- The retry timing of Meta after a `503`, and the size limit of a batch.
- Media download from Meta, and the real size and host of each file.

## The question to a member

The number sends one kind of message: the question to a member who forwarded messages that name no author. It
asks "Who wrote this?" in the member's chat with the number, and the member answers there. The question is a free-form
text, so Meta allows it only within 24 hours after the member last wrote. After that the batch stays held until it
expires. The number sends nothing to a customer, and a reply into a customer's conversation is still refused.

`whatsapp_business_messaging` must be on the token. A token without it answers the question with an error that
Margince treats as a refused credential. The batch then stays held, and the question is tried again after a pause.
