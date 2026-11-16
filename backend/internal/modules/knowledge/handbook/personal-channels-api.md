<!-- prose:plain -->
<!-- describes: backend/internal/modules/personalchannels backend/internal/compose/personalchannelwire backend/api/crm.yaml -->
# Connect a chat client with the Personal channels API

A client of your own, such as Chat Bridge, can send a member's chats and files into Margince and pick up the replies they write. Every call goes from the client to Margince, so the client needs no public address. This page is for the person who builds or runs that client.

### What does a client need to call the Personal channels API?
A client needs a passport with the **Personal channels** permission, and the member's personal channels turned on.
1. The member opens **Settings → Connections** and chooses **Turn on** in **Personal channels**.
2. In **Settings → AI assistants**, **For developers**, the member chooses **New passport for Chat Bridge**. The form opens with **Personal channels** ticked. **Never expires** is offered under the lifetime.
3. The client sends the passport as `Authorization: Bearer <passport>` on every call.
A passport with only **Personal channels** can send chats in and pick replies up, and nothing else. It cannot read or change other records.

### How does a client send messages in?
A client sends messages with `POST /v1/personal-channels/chats`. The body holds 1 to 50 messages. Send an `Idempotency-Key` header, so a request you send again lands nothing twice.

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
- `network` names the chat app: lower-case letters, digits and `_`, at most 32. No network means `generic`. Any new name works with no change in Margince.
- `direction` is `outbound` for a message the member sent from their phone; `from` is then the member.
- A party has an `account` and a `name`, and never an email address. An account may not contain `:` or `/`.
- `group_id` names the group a message was sent in, if any.

The answer has one result per message, in the order sent: `landed` with its `activity_id`, `duplicate`, `skipped` (capture did not keep it, for example an erased contact), `refused` with a `reason`, or `deferred` (the request ran out of time; send those again). One refused message does not refuse the rest.

### How does a client send a file?
A client stages each file with `POST /v1/personal-channels/uploads` as a multipart form field named `file`, then names its `upload_id` in the message's `files`. A file may be 25 MB. The answer gives the type read from the bytes, the size and the SHA-256. A staged file no message names within 24 hours is deleted. At most 50 files, or 200 MB, may wait staged at once.

### How does a client pick up replies?
A client picks up replies with `POST /v1/personal-channels/replies/pull` and a body such as `{"max": 20}` (1 to 50). Each reply names its `delivery_id`, `network`, `recipient.account`, `group_id`, `reply_to`, `body` and `attempt`. The answer also holds the connection's state: whether it is on and how many replies wait.
A pulled reply is held for 5 minutes. If the client does not acknowledge it by then, the next pull offers it again with `attempt` one higher.

### How does a client say a reply was sent?
A client acknowledges with `POST /v1/personal-channels/replies/ack`:

```json
{"acks": [
  {"delivery_id": "019a...", "outcome": "sent", "provider_message_id": "wamid.HBgM..."},
  {"delivery_id": "019a...", "outcome": "failed", "reason": "the contact blocked this number"}
]}
```

Each answer is `recorded`, `unknown_delivery` or `already_settled`. A `failed` reply shows as **Not delivered** on the timeline and is not tried again; the member sends it again. A reply nobody acknowledges within 7 days is marked **Not delivered** too.

### What do the error answers mean?
- `409` with "personal channels are off": the member must turn them on in **Settings → Connections**.
- `403`: the passport lacks the **Personal channels** permission, or the member's role does not hold it.
- `422`: a field is wrong; the answer names it.
- `429`: too many calls. Wait and send again.

### Can an AI assistant use the same calls?
An AI assistant connected over MCP uses the `personal_channels` tool, with the actions `push_messages`, `upload_file`, `pull_replies` and `ack_replies`. Through MCP a file can be about 6 MB.
