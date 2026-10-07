<!-- prose:plain -->
# Connect a Telegram bot

Bind a Telegram bot, and Margince captures the messages customers send it into the timeline. It creates
contacts and activities through the one place that finds copies, and a rep can reply from that timeline.
You do all of it in the app. The REST surface behind it is the contract (`backend/api/crm.yaml`,
`/channel-connections*`), and nothing below needs you to call it by hand.

For the side that takes messages in (the connector seam, the one Sink, how the credential is kept), read
[explanation/capture-connectors.md](../explanation/capture-connectors.md). For the side that sends (the
staging row, the gates, the dispatcher), read
[explanation/outbound-messaging.md](../explanation/outbound-messaging.md).

> **One company per installation.** One installation serves one company, and the server works out which
> one on its own, so nothing you do here picks a tenant. `channel_connection` has no tenant column at all,
> and core tables have no row-level security. "The bot" below means the one bot of the installation.

## What kind of connection this is

A bot binding is **not** a mailbox. A mailbox (**Settings → Connections → Connected inboxes**) is the
grant of one human over their own mail. A Telegram bot is an **admin who binds one bot for every user**.
That has these results:

- **One live bot, and no more.** `uq_channel_connection_provider` is a unique index over only the live
  rows, keyed on `(provider)`. Every reply that goes out looks up the bot of the installation, so with two
  live bindings the send path refuses to guess. A second bot would remove the way to reply on either
  one.
- The card holds this rule on the screen too. Once a bot has a binding, the **Connect a Telegram bot** button
  is no longer there. That leaves **Replace token** and **Disconnect** as the only actions.
- **Admin and ops bind it; all read it.** The RBAC grants for `channel_connection` give create,
  update and delete to `admin` and `ops` only. `management`, `manager`, `rep` and `read_only` all hold
  read. A rep needs to know whether the channel is live before they expect a reply to come there.
- Every action is also `x-agent-access: human-only`. The bot token can read every message the bot gets,
  so an agent must never bind one on its own.
- **The customer writes to the bot first.** A Telegram bot cannot start a chat. A contact can only be
  reached once a message from them binds a channel identity for them. So the **How to send** list in the
  composer offers Telegram only for someone who already has a chat on it. It labels it
  `Continues your Telegram conversation`.

A mailbox connector has several things that this one does not. There is no OAuth app, no consent
redirect, no callback URI, and no HTTP route for messages that come in. **Messages come in by long poll**: this
installation calls out, and nothing ever calls in.

## Before you start

You need two things:

- **A bot token from BotFather.** Write to [@BotFather](https://t.me/BotFather) on Telegram, send
  `/newbot`, and keep the token it gives you. It has the shape `<bot id>:<secret>`. The server checks
  that shape before it spends a network call on it. So when you paste the *user name* of a bot, it comes
  back refused, without a call.
- **The vault key**, `MARGINCE_KEYVAULT_ROOT_KEY` (`base64` of 32 bytes), on **both** the API and the
  worker. The API seals the token on connect, and the worker opens it on every poll.

Without a vault, the card shows `Messaging channels aren't configured in this deployment.`, and no
connect button. The API refuses every path that changes data, by name, so it does not store a token that
nothing could open. And the worker registers **neither** Telegram job. Both `telegram_poll` and
`telegram_poll_sweep` declare `registration: {when: [ChannelVault], absent: registers_nothing}`.

```sh
export MARGINCE_KEYVAULT_ROOT_KEY="$(openssl rand -base64 32)"  # base64 of exactly 32 bytes
```

This surface does **not** need or read any of these:

| Not needed | Why |
|---|---|
| An OAuth app, or a client ID and secret | There is no consent step; the token *is* the credential. |
| `MARGINCE_CONNECTOR_STATE_KEY` | There is nothing to sign: no redirect, and no state that goes out and comes back. |
| `MARGINCE_PUBLIC_BASE_URL` | Nothing needs to know where to reach this installation. |
| A callback or redirect URI | The same. `WithChannelSurface()` takes no deployment config at all. |
| A webhook route for messages that come in | The poller calls `getUpdates`. The connect step **clears** any webhook the bot already has, because Telegram refuses `getUpdates` while one is registered. |

> Start again with `make dev` after you set the vault key.

## Connect

1. Open **Settings → Connections**. The **Telegram bot** card is below **Connected inboxes**.
2. Click **Connect a Telegram bot** on the card.
3. Paste the BotFather token into **Bot token**, which is a password field.
4. Click **Connect**. When it works, the panel reads `Connected as @yourbot.`, with a status badge and a
   **Done** button.

The card has the line `One bot receives and sends messages for the whole company.` under its title. With
no bot, its row reads `No bot is connected yet.` The help text on the token field says
`Paste the token BotFather gave you when you created the bot. We seal it in the credential vault and never show it again.`

The form keeps nothing after a failed send, so when you try again you start from an empty box. After it
connects, the card behind the panel reads the list of connections again to prove the binding. So it never
claims one that the server did not confirm.

The `telegram_poll_sweep` dispatcher of the worker runs every `30s`. On its next turn it picks the
binding up, and starts a long poll on it. The cursor starts at `0` ("whatever Telegram still holds"), so
a new bot gets the messages that already wait for it.

**A failed connect leaves no part done.** Connect runs in a fixed order:

1. `getMe` checks the token, and returns the bot ID and the @ user name.
2. `deleteWebhook` clears any webhook that is registered. Connect does not send `drop_pending_updates`, because those
   waiting updates are the messages of the customer.
3. Margince seals the token in the vault.
4. Margince adds the row as `connected`, with a cursor of 0, in **one transaction with its audit row**.

Nothing comes after that write, because a poll calls out, and there is nothing to register. No server
makes the `pending` status of the schema. The screen still shows it as
`Pending — not yet confirmed live` if one comes from an older server, or a server from another source. So it
never looks live.

A failure at any step before the commit leaves nothing behind but a vault entry. The path deletes that
entry itself when another write takes the unique index first.

## What the card shows once a bot has a binding

It shows one row: `Telegram · @yourbot`, a status badge, and the two actions that change it.

| Badge | Status | Meaning |
|---|---|---|
| **Capturing** | `connected` | Live. Polled on every turn of the dispatcher. |
| **Needs reconnect** | `reauth_required` | Telegram refused the sealed token. No retry can repair it; use **Replace token**. |
| **Sync error** | `error` | Another reader holds the updates of this bot. Find it and stop it: a second installation, a staging stack, or some other integration. |
| **Disconnected** | `disconnected` | Archived, and not in the list. |

Margince does not poll either stopped status again until an operator does something. The scan for due rows picks
only `connected` rows, and that ends the retry loop. The audit log records the reason in
`poll_stopped_because`, because the row itself has no column for it.

## Rotate the token

Click **Replace token** on the row, paste the new token, and send it. The panel has the title
`Replace the bot token`. It keeps the **current status badge of the connection** on the screen while you work.
So a binding that the poll has stopped still reads as stopped while an edit form is open on it.

To rotate happens **in place**; it is never a disconnect and then a reconnect. Here is what stays, and
what starts again:

- **The row stays**, and with it every channel identity binding and all captured history. Telegram user
  IDs are the same for all bots, and the unique key of the identity leaves out the bot ID. So identities
  still match after you rotate, even after a swap to a *different* bot.
- **The poll cursor starts again** (`poll_offset = 0`). `update_id` is a count for each bot. Say the new bot
  kept the place of the old bot in that count. Then the poll would ask it for numbers past any it has
  sent. Every message sent to it would be skipped, and no one would know.
- **The connection is always live.** A poll calls out, so only the row decides which token the next poll
  uses. To point the row at the new token is the whole change.
- **Margince deletes the old token** from the vault once the row names the new one. Nothing else would
  ever remove it.
- Margince clears the webhook of the new bot, for the same reason connect clears one. The old bot needs
  nothing: it stops being polled as soon as the row stops naming it.

Say you rotate while a poll or a send is in the middle. A fence keeps it safe. The step where the poll
moves its place forward carries a `channel_id = <the bot it actually spoke to>` check. The send path
reads the version of the binding again right before it uses the credential. If they do not match, it refuses for
a short time, so the delivery looks up the binding again and does not stop.

## Disconnect

Click **Disconnect** on the row, then confirm `Disconnect this bot?`. The text says:
`This deletes the stored token and stops checking the bot for new messages. Capture and sending stop immediately; everything already captured stays in your CRM.`

Three kinds of state have three different results:

| | What happens |
|---|---|
| The binding row | **Archived**, status `disconnected`. That stops the poll (the scan for due rows picks only live `connected` rows). It also frees the unique index on live rows, so the same bot, or another one, can connect here again later. |
| The bot token | **Deleted** in the vault. To take back a connection removes the credential, and the row too. |
| Captured activities, contacts, channel identities | **Kept.** To disconnect stops capture; it does not erase history. (To erase is the job of Art. 17; see [explanation/privacy-and-consent.md](../explanation/privacy-and-consent.md).) |

Like connect and rotate, this needs the vault. Without one it refuses, so it does not archive a row whose
sealed token nothing could then delete.

## Check it from end to end

1. **The bot has a binding.** **Settings → Connections** shows the Telegram row as **Capturing**.
2. **A customer message turns into an activity.** From a **second** Telegram account, open a private chat
   with the bot and send a message.
3. **The contact was created.** The Sink sends the sender through the one place that finds copies, in
   the contacts module. It does the same for the sender of a mail.
4. **Grant consent.** Open the contact, find the **Consent** section, and **Grant** the purpose you mean
   to send under.
5. **Reply from the timeline.** Click **Reply** on the entry from the customer, pick a **Consent purpose**, and
   confirm.
6. **The reply is filed on its chat.** The activity that goes out carries the `thread_key` of the first
   message.
7. **The token never shows.** No read returns it: not the list, not the connect response, and not the
   audit log.

More on step 2: within about one turn of the dispatcher, the message appears on the timeline as an
activity. It has `kind: message`, `channel_provider: telegram` and `direction: inbound`, and the stamp
`connector:telegram`. Every captured channel message files as the one `message` kind, and
`channel_provider` names the transport. So to ask for Telegram means `channel_provider=telegram`. A
message with media and no words reads as a short label inside `[ ]`, such as `[photo]` or
`[voice message]`. So the
timeline still shows that the customer wrote.

More on step 3: the contact has **no owner**. A bot for the whole workspace works for no one human, and the
`connected_by` of the connection is for the audit only. So the admin who connected the bot never becomes
the owner. **Margince makes no company**, because a channel identity has no mail domain to make one from.

More on step 4: some purposes need a double opt-in token first. Sending is refused by default *for each
purpose*: a grant for one purpose never allows another.

More on step 5: **Reply** opens the composer with no Subject and no Cc, because a channel has neither. It
asks for a **Consent purpose**. You confirm at `Send this message?`, which says
`You are sending this message now. This is an outbound, irreversible action.`

- **You never name who gets the message.** The entry you replied to *is* the chat, and its
  `channel_provider` names the channel. The server finds the channel identity of the contact that chat is
  with. The request carries only the body, the consent purpose, and if you want, files already stored
  in Margince. It names them by ID, and you do not add them here.
- Without a grant, Margince holds the send back, and the composer says so:
  `Send blocked — no consent`, with a **Review consent** link back to the contact. That is why the
  surface shows a list of purposes to pick from.
- **Reply does not appear** for a contact the channel cannot reach. That is an identity with no binding,
  or one that Telegram has reported as blocking the bot. Margince does not offer a button that could only fail.

More on step 6: so the reply check of capture matches the next message from the customer against it. It
sends the `engagement.reply` event, which names `telegram` as the channel.

More on step 7: the audit images carry the provider, bot ID, label and status, and never a vault reference.

## When connecting fails

Every refused call answers with RFC 7807, and a fixed `detail`. The panel shows that `detail` word for word, in a warning
box, and does not turn it into `couldn't connect`. The `description` text from Telegram never reaches
the wire: it stays in the error that the server logs. The `code` column is what you will find in
a log or an audit row.

| What the warning box tells you | `code` (status) | What to do |
|---|---|---|
| The token was refused, or cannot be a BotFather token at all | `channel_token_rejected` (400) | Check the token BotFather gave you, and that no one revoked it. |
| A bot is already connected here | `channel_workspace_already_bound` (409) | Disconnect it first, or use **Replace token** to point the binding at a different bot. |
| Someone else changed this connection while it was open for you | `version_skew` (409) | Open the card again and try again; nothing was written. |
| Telegram could not be reached | `channel_provider_unreachable` (502) | Nothing was changed; try again once the provider is back. |
| Telegram read the request and refused it | `channel_provider_rejected` (502) | Nothing was changed; check that no one has limited or deleted the bot in BotFather. |
| No credential store is set up | `channel_credentials_not_configured` (503) | Set `MARGINCE_KEYVAULT_ROOT_KEY`, and restart. |
| `Messaging channels aren't configured in this deployment` | `channel_connections_not_configured` (503) | This server role has no channel store, and says so with a 503 and not a 500. `cmd/api` serves it. |
| You may not change this | `permission_denied` (403) | Every role can read; only admin and ops can bind. |
| No such connection | `not_found` (404) | This keeps it secret whether the row exists: an archived binding reads the same. Open the card again. |
| (no warning box) | `conflict` (409) | This binary has no code for that provider, or some unique index other than the rule on live rows refused the write. Margince has code only for `telegram`. This is the last answer, and adds no second rule for bindings. |

## Limits

- **No backfill, ever.** The Bot API has no history endpoint. Telegram keeps updates that no one has read
  for only about 24 hours, so there is nothing to page back through. The Telegram connector is not a
  `Backfiller`; capture starts when you connect.
- **Messages wait up to one dispatcher turn.** `telegram_poll_sweep` runs every `30s`, and
  queues one `telegram_poll` job for each live binding. Each job holds a long poll open for 25 seconds.
  The time limit of a `telegram_poll` job is `2m`, which must be more than the long poll plus some time
  for the client.
- A poll that comes back *with* updates ends its job. So when many messages wait, they clear at one Bot API
  answer per turn. Telegram allows one reader per bot. The rule that keeps one job per bot is declared on
  the type of the job values (`TelegramPollArgs.InsertOpts`). So no code that adds a job can leave it out.
- **A block is a problem with the user.** Telegram answers `403` for
  "bot was blocked by the user", and for an account that is turned off. A staged delivery **stops at
  once**, and does not spend the retry ladder. The reason says that to try again or to reconnect the
  channel both change nothing.
- On its own path, Telegram reports the block as a `my_chat_member` update. That sets `blocked_at` on the
  channel identity of the contact, and from then on Margince does not offer the **Reply** button at all.
  When the user ends the block, that clears it. The `update_id` of the update sets the order, so two
  changes cannot apply in the wrong order.
- **Private chats only.** Margince refuses messages from group and `supergroup` chats before it stores any data.
  A bot in a group runs in the default privacy mode of Telegram, and would see only parts of the chat. A
  reply finds who gets it through the *private* chat of the sender. So a message filed from a group could
  never be answered where it started.
- **Margince names media; it does not get files.** The body carries the text or the
  `caption`. A message
  with no words reads as `[photo]` / `[document]` / `[attachment]`. To get the file itself is out of
  scope.
- **A reply goes to the whole chat.** So a channel delivery that goes out is staged on
  its own. It does not guess at how the capture provider keys its messages.

## Where the code lives

| | |
|---|---|
| The workspace binding: the order of connect, the write shape, the RBAC gate | `backend/internal/modules/capture/channelconn.go` |
| Rotate and disconnect (point in place, archive, delete the credential) | `backend/internal/modules/capture/channelconnedit.go` |
| The `/channel-connections` transport, and how it maps wire codes | `backend/internal/modules/capture/handlers_channel.go` |
| The reads and writes of the poller: due scan, poll target, cursor move, stop | `backend/internal/modules/capture/channelpoll.go` |
| How a send finds the bot, and the fence for a new token | `backend/internal/modules/capture/channelsend.go` |
| How a channel contact is created, and how it waits for an erase | `backend/internal/modules/capture/sinkchannel.go` |
| Where Margince meets the Bot API, the error values, the token shape check | `backend/internal/modules/capture/telegram/api.go`, `auth.go` |
| How an update turns into an activity, and how a block, or its end, is read | `backend/internal/modules/capture/telegram/normalize.go`, `membership.go` |
| The registered connector, and its `MessageSender` seam | `backend/internal/modules/capture/telegram/send.go` |
| Setup: the connect surface, the poll dispatcher and worker, the worker that takes messages in | `backend/internal/compose/channelconnect.go`, `telegrampoll.go`, `telegrampollscope.go`, `telegramingest.go` |
| Whether a contact can be reached (`blocked_at`), and the identity binding | `backend/internal/modules/contacts/channelidentity.go` |
| The reply and its rules: how it finds who gets it, the order of gates, staging | `backend/internal/modules/activities/channelsend.go` |
| The tables | `channel_connection`, `contact_channel_identity`, the channel rows of `erasure_suppression`, and the channel shape of `comms_outbound` |
| The REST contract | `backend/api/crm.yaml` (`/channel-connections*`, `/activities/{id}/send-message`) |
| Where jobs are declared | `backend/api/jobs.yaml` (`telegram_poll_sweep`, `telegram_poll`, `telegram_ingest`) |
| The card, the panel to connect or replace, and the words for each status | `frontend/src/screens/connectors.tsx`, `telegram-connect-form.tsx`, `connector-status.ts` |
| The reply composer, and the **Reply** button on the timeline | `frontend/src/screens/compose.tsx`, `contacttransports.ts` |

## Where to go next

- The seam for messages that come in (the one Sink, the one place that finds copies, how the credential is
  kept): [explanation/capture-connectors.md](../explanation/capture-connectors.md).
- The side that sends (the staging row, the seat and consent gates, the dispatcher, at most once):
  [explanation/outbound-messaging.md](../explanation/outbound-messaging.md).
- To connect a mailbox in place of a bot (Gmail, IMAP, Graph, Calendar):
  [how-to/connect-a-mailbox.md](connect-a-mailbox.md).
- The consent model that the reply gate reads:
  [explanation/privacy-and-consent.md](../explanation/privacy-and-consent.md).
- Every flag and environment value, with the vault key:
  [reference/configuration.md](../reference/configuration.md).
