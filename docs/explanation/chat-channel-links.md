<!-- describes: backend/internal/modules/chatlinks backend/internal/compose/chatsync backend/internal/compose/chatsummary -->
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

When an activity's audience narrows after it was posted, its item is taken out. The next flush rewrites
that post without it. The deal value stays off the card, because it is hidden from members who may not
see it.

## Messages from the chat itself

An activity the chat delivered (`source_system = dispact`) is never posted back, so a note filed from a
channel cannot loop into the same channel.

## From the channel's talk to a note

The job `chat_pull_sweep` runs every minute and enqueues one `chat_pull` for each live Dispact bot. The
pull is unique per bot while it waits or runs, so any caller that wants a channel read again enqueues the
same job.

For each active link, the pull reads the channel's history after the link's `history_cursor`, at most
five pages of 99 messages. It skips the bot's own posts and the chat's `system` and `member_join`
notices, and talk from before the link was made. Then it cuts the messages into episodes:

- An episode is a run of messages with less than 30 minutes between them, all on one day (UTC). A
  thread's replies are talk like any other, so a thread answered within the half hour stays in its
  episode.
- An episode is closed when the next message starts another one, when its last message is 30 minutes
  old, or when its day has ended.

Each closed episode is judged in order by the `chat_summary` task's `episode` site
(`compose/chatsummary`). The messages are read again from the chat for this and are held only in
memory. When the model says the talk matters, the title and summary are filed as one note on the
link's deal and project, under the system actor `system:chat_links`. The note's
`(source_system, source_id)` is `dispact` and `<channel slug>:<first seq>`, which the activity table
holds unique, so a replay or a second link to the same channel files nothing twice.

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
the record's own channel. Filing the same message again returns the note already filed.
