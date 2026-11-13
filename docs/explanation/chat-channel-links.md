<!-- describes: backend/internal/modules/chatlinks backend/internal/compose/chatsync -->
# How chat channel links work

A deal or a project can be linked to one private channel of a team chat (Dispact). Margince keeps a
card in the channel and posts what happens on the record. The posts are built from the live records,
and only from what the whole workspace may read. To set it up, read
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
