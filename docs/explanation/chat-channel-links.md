<!-- describes: backend/internal/modules/chatlinks backend/internal/compose/chatsync backend/internal/compose/chatsummary backend/internal/compose/dispactapp backend/internal/platform/lastseen -->
# How chat channel links work

A deal or a project can be linked to one private channel of a team chat (Dispact). Margince keeps a
card in the channel and posts what happens on the record. The posts are built from the live records,
and only from what the whole workspace may read. In the other direction, Margince reads the channel's
talk and files a short AI summary on the record when the talk matters. To set it up, read
[connect-dispact.md](../how-to/connect-dispact.md).

## Two tables, and why only two

- `chat_link` is one record's channel: the channel, the level it posts at, the card it keeps, and how far
  the channel's history has been read.
- `chat_post_item` is one event bound for a channel, keyed on `(link_id, event_id)`. A replayed event
  adds nothing.

A post has no table of its own. The items one post carried share its id and the chat's message id, and
that is enough to rewrite the post later. The bot connection reuses `channel_connection`, beside
Telegram. No chat text and no activity text is stored in either table. A post is built from the live
records when it is sent.

## One deal, one project, one channel

A deal and its project share a channel. Linking a deal whose project has no channel also links the
project. When a deal gets a project later, the deal's channel takes the project in, unless that
project already has a channel of its own.

## From an event to a post

1. The consumer `cg:chat-links` finds the deals and projects an event touches. An offer or contract
   reaches its deal. An activity reaches the records it is linked to. A deal reaches its project.
2. Each event has a tier. Stage, offer, contract and phase steps are **milestones**. Owner changes, the
   value, close date and name of a deal, and notes, meetings and mail are **important**. The rest is
   **everything**. A test fails when the catalog gets an event that has no tier and is not ignored.
3. The link's level decides which tiers it keeps: `everything`, `important` (the default), `milestones`,
   or `silent`, which keeps none. Every event still schedules a flush, so the card stays current.
4. The flush waits two minutes, so many changes in a row become one post.

## Who may read what reaches the channel

A channel is read by everyone in it, guests included. So an activity reaches it only when the whole
workspace may read it. The function `sharedActivity` decides this, and a test fails when the engine
reads activity text in any other place. It is asked when the event is recorded and again right before the
post is sent.

When an activity's audience narrows after it was posted, or it is archived or restricted, its item is
taken out. The next flush rewrites that post without it. A restriction (`retention.restricted`) rides the
contact stream, so `cg:chat-links` reads that stream too. The events that narrow an activity are one list,
`events.NarrowsActivity`, which the voice-copy recheck reads as well. The deal value stays off the card,
because it is hidden from members who may not see it.

## Who a new channel is opened to

A channel Margince creates is opened to every live member who may open the record, at most 50. The
record's owner and the member linking it come first. Each member is judged by the record's own read
admission (`auth.AdmitReaders`) under their live grants, in `chatsync.Viewers`. Members over the 50 are
counted as `skipped_over_cap`, and members the chat does not know as `not_in_chat`.

## Messages from the chat itself

An activity the chat delivered (`source_system = dispact`) is never posted back, so a note filed from a
channel cannot loop into the same channel.

## From the channel's talk to a note

The job `chat_pull_sweep` runs every minute and enqueues one `chat_pull` for each live Dispact bot. The
pull is unique per bot while it waits or runs, so any caller that wants a channel read again enqueues the
same job.

### Push, then pull

A Dispact notice carries no message text. It only says that a channel has news, so the receiver
(`chatsync.Receiver`) checks the signature and enqueues that same `chat_pull`. The pull then reads the
history after the cursor, as it does on a timer. Ten notices while one pull waits make one pull. A
notice that comes while the pull runs finds the job still running and adds nothing, so the pull checks
when it ends whether a notice came after it started, and runs once more if one did.

`channel.archived` pauses the channel's link through `MarkChannelGone`, the same write a `404` from the
chat makes. A notice is matched only to the links of the connected bot, and its `workspace_id` must be
that bot's team, so a notice for another team cannot reach this installation's links.

### The cadence switch

The function `SweepDue` decides when the sweep reads a bot. Each valid notice records its time. While
the last one is under an hour old, the sweep reads the bot only when an hour has passed since the sweep
last read it. That hourly read catches what a lost notice missed and closes episodes whose quiet time
ended with no new message. Once the last notice is an hour old, the sweep reads every minute again. A
broken push therefore falls back to polling without anyone acting.

The two times live in Redis with a time to live of one hour (`platform/lastseen`), not in a column. A
notice can come every two minutes per busy channel, and a write per notice to `channel_connection`
would cost a row version, a WAL record and an audit question each time. A Redis key costs none of that,
and losing it only means the sweep polls every minute until the next notice. When the worker cannot
read Redis, the sweep reads every minute.

For each active link, the pull reads the channel's history after the link's `history_cursor`, at most
five pages of 99 messages. It skips the bot's own posts and the chat's `system` and `member_join`
notices, and talk from before the link was made. Then it cuts the messages into episodes:

- An episode is a run of messages with less than 30 minutes between them, all on one day (UTC). A
  thread's replies are talk like any other, so a thread answered within the half hour stays in its
  episode.
- An episode is closed when the next message starts another one, when its last message is 30 minutes
  old, or when its day has ended.
- When a read stops at the page cap, its last episode stays open, because its talk may go on in the
  next page. An episode that alone fills the whole read is filed anyway, so one very long day cannot
  stall the link.

Each closed episode is judged in order by the `chat_summary` task's `episode` site
(`compose/chatsummary`). The messages are read again from the chat for this and are held only in
memory. When the model says the talk matters, the title and summary are filed as one note on the
link's deal and project, under the system actor `system:chat_links`. The note's
`(source_system, source_id)` is `dispact` and `<channel slug>:<first seq>`, which the activity table
holds unique, so a replay or a second link to the same channel files nothing twice. No other writer
may stamp `source_system = dispact`, so no one can claim a summary's key before it is written. The note
is filed in one transaction that first checks the link is still live and still names the same deal and
project, so an unlink during the model call files nothing.

After each episode the cursor moves to its last message, under the condition that it still holds the
value the pull read. A second pull that moved it first wins, and this one stops. The first open episode
stops the pull, so the cursor never passes talk that was not judged. An irrelevant episode files nothing,
and the cursor moves past it. With no model bound, or an answer the site refuses, the episode is passed
over the same way, so a channel never waits for a model. A model that asks to wait stops the pull until
the next tick.

No episode is stored. The cursor marks the end of what was judged, and the note keeps only its
summary. Its `raw` payload holds the message count, the time of the last message and a link to the
first one, which the API serves as `chat_episode`. Undo is archiving the note: the cursor never goes
back, so nothing is filed again.

## The talk so far

Linking an existing channel asks `summarize_history`. When it is true, the link has
`backfill_requested`, and its first pull reads the talk from before the link instead. It judges only the
five latest closed episodes, skips the older talk, moves the cursor to where the link began, and clears
the request. A long history is read 50 pages a tick. A channel Margince created never asks.

## Filing one message

`POST /channel-links/{record_type}/{id}/notes`, and the MCP action `file_channel_message`, file a summary
of one message, or of the message and the replies in its thread that follow it, as the caller. The
message is named by its seq, `#slug:seq`, the chat's `<m:#slug:seq|…>` mention or its link, and must be in
the record's own channel. Filing the same message again returns the note already filed. A thread longer
than five pages answers `422` `channel_thread_too_long` and files nothing, because a part of it would
take the thread's key for good.

## The Margince app in Dispact

The `@Margince` agent in Dispact calls the same `/mcp` an MCP client calls, but with a different
credential. Dispact signs a short token for each call, naming the chat user who asked. The package
`compose/dispactapp` checks it in front of the passport path:

- The token must come from the connected bot's address, for its team, with a 60 to 120 second life and
  an id not seen before. The keys come from the address's published key set, through the same
  `oidctoken` verifier that checks Google and Microsoft sign-in tokens.
- The chat user is matched by their confirmed email to a live member, once. That member gets a passport of
  their own, and the passport keeps Dispact's issuer and subject (`passport.app_issuer`,
  `passport.app_subject`). Later calls find the passport by that pair. No other table holds the
  binding.
- So the agent never acts with more than that member could do in the app. The member's live grants and
  the passport's scopes decide every call, as for any agent. Revoking the passport ends it, and
  the revoked passport keeps refusing that chat user. Releasing it (`DELETE /passports/{id}/app-binding`,
  **Allow again** in Settings) clears the binding, so their next call mints a fresh passport.

Dispact reads the tool list once and shows it to every user, so the list cannot depend on who asked.
The edge gives the request the fixed list `dispactapp.Tools`. `tools/list` returns those tools and no
others, and `tools/call` refuses any other name as an unknown tool. A chat user who is no member may read
that list, and gets `403` `no_margince_seat` for anything else.
