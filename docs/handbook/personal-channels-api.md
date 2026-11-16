<!-- prose:plain -->
<!-- describes: backend/internal/modules/personalchannels backend/internal/compose/personalchannelwire backend/api/crm.yaml -->
# Connect a chat client with the Personal channels API

A client of your own can send a member's chats and files into Margince and pick up the replies they write. Every call goes from the client to Margince, so the client needs no public address. This page is for whoever builds or runs that client.

### What does a client need to call the Personal channels API?
A client needs a passport with the **Personal channels** permission, and the member's personal channels turned on.
1. The member opens **Settings → Connections** and chooses **Turn on** in **Personal channels**.
2. In **Settings → AI assistants**, **For developers**, the member chooses **New passport for Chat Bridge**. The form opens with **Personal channels** ticked. You may tick **Never expires**.
3. The client sends the passport as `Authorization: Bearer <passport>` on every call.
A passport with only **Personal channels** can send chats in and pick replies up, and nothing else. It cannot read or change other records.

### How does a client send messages in?
A client sends messages with `POST /v1/personal-channels/chats`. The body holds 1 to 50 messages. Send an `Idempotency-Key` header, so a request you send again lands nothing twice. Each message's `message_id` also lands it once.

```json
{"messages": [{
  "message_id": "wamid.HBgL...",
  "network": "whatsapp",
  "direction": "inbound",
  "occurred_at": "2026-11-16T09:00:00Z",
  "body": "Here is the signed quote",
  "from": {"account": "84901234567@s.whatsapp.net", "name": "An Tran"},
  "to": {"account": "me", "name": "Me"},
  "files": [{"upload_id": "019a..."}]
}]}
```

- `message_id` is the chat app's own id. A message sent again with the same id answers `duplicate`.
- `network` names the chat app: lower-case letters, numbers and `_`, at most 32. No network means `generic`. Any new name works with no change in Margince.
- `direction` is `outbound` for a message the member sent from their phone; `from` is then the member. A reply your client confirmed as sent, sent in again as `outbound` with its `provider_message_id` as the `message_id`, answers `duplicate`.
- `from` and `to` each have an `account` and a `name`, and never an email address. An account may not hold `:` or `/`.
- `group_id` names the group a message was sent in, if any.

The answer has one result per message, in the order sent. One refused message does not refuse the rest.
- `landed`, with its `activity_id`.
- `duplicate`: it landed before.
- `skipped`: capture did not keep it, for example for an erased contact.
- `refused`, with a `reason`.
- `deferred`: the request ran out of time. Send those again under a new `Idempotency-Key`.

### How does a client send a file?
A client stages each file with `POST /v1/personal-channels/uploads` as a form field named `file` (`multipart/form-data`), then names its `upload_id` in the message's `files`. A file may be 25 MB, and one message may name 20 files, 50 MB together. A file is used once: a message that lands takes it off staging. The answer gives the type read from the file itself, the size and the SHA-256. A staged file no message names within 24 hours is deleted. At most 50 files, or 200 MB, may wait staged at once.

### How does a client pick up replies?
A client picks up replies with `POST /v1/personal-channels/replies/pull` and a body such as `{"max": 20}` (1 to 50). Each reply names its `delivery_id`, `network`, `recipient.account`, `group_id`, `body` and `attempt`. The answer also holds the connection's state: whether it is on and how many replies wait.
A reply the client picked up is held for 5 minutes. If the client does not confirm it by then, the next call offers it again with `attempt` one higher.

### How does a client say a reply was sent?
A client confirms with `POST /v1/personal-channels/replies/ack`:

```json
{"acks": [
  {"delivery_id": "019a...", "outcome": "sent", "provider_message_id": "wamid.HBgM..."},
  {"delivery_id": "019a...", "outcome": "failed", "reason": "the contact blocked this number"}
]}
```

Each answer is `recorded`, `unknown_delivery` or `already_settled`. A `failed` reply shows as **Did not arrive** on the timeline and is not tried again; the member sends it again. A reply nobody confirms within 7 days is marked **Did not arrive** too. A confirmation still counts while personal channels are off.

### What do the error answers mean?
- `409` with "personal channels are off": the member must turn them on in **Settings → Connections**.
- `403`: the passport does not have the **Personal channels** permission, or the member's role does not hold it.
- `422`: a field is wrong; the answer names it.
- `429`: too many calls. Wait and send again.

### Can an AI assistant use the same calls?
An AI assistant connected over MCP uses the `personal_channels` tool, with the actions `push_messages`, `upload_file`, `pull_replies` and `ack_replies`. Through MCP a file can be about 6 MB.
