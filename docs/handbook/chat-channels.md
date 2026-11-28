<!-- prose:plain -->
<!-- describes: frontend/src/components/channelpanel.tsx frontend/src/components/channelsyncsettings.tsx frontend/src/components/channeloutboundpreview.tsx frontend/src/components/channeldeliveryreview.tsx frontend/src/components/channelsyncended.tsx frontend/src/features/deals/dealdeliveryhandoff.tsx frontend/src/features/settings/channelinstallations.tsx frontend/src/design-system/timelinechat.tsx backend/internal/modules/chatlinks backend/internal/compose/chatsync -->
# Team chat channels

Use **Team chat** on a company's, deal's or project's Overview to connect a work channel in Dispact,
Slack or Teams. Channel sync saves relevant channel decisions to History and can send allowed CRM
updates back to that channel.

[Messenger](messenger.md) is where you write a quick message yourself.

## Link a work channel

An administrator connects the shared provider under **Settings**, **Integrations**. Connect your own
messaging account there too. Shared installation and personal access are separate: adding a bot
does not give it your private conversations.

On the record, choose the provider and link an existing channel. Review the channel and its
scope. A shared room needs a clear thread so other work does not become this record's
history. Where the provider supports it, you can create a private channel from the same card.
Private Teams channels support incoming summaries; automatic sent posting is not available there.

A channel can belong to a company even when no deal or project exists. Separate threads in a shared
room can serve separate records. Linking a company does not give its readers access to a hidden
deal or project.

Slack installation asks the account connecting it for read-only channel history, separately from personal Messenger. That account must be a member of the linked private channels. It asks for no private message history or personal sending permission. Losing that access pauses incoming summaries while the shared bot can still send eligible CRM updates. Reconnect the shared installation to renew consent.

Slack saves how the app is shared, as confirmed by an admin, and the documented history limits. Complete summaries require an internal app or one approved for the Slack app store, with the usual history limits. Other commercial apps show that complete summaries are paused; personal Messenger and CRM posts are separate. Admins can review the saved limits and confirmation time in Settings or the link’s Manage details. Provider retry delays still apply.

## Choose what to save and send

Open **Manage** on Team chat. **Save channel updates to History** and **Send CRM updates to channel**
are separate switches. New links start with CRM posts off. Turning reads back on starts with new
channel activity rather than importing the gap.

Before enabling CRM posts, **Review channel delivery** shows the current audience, current
overview card and allowed queued updates. It also shows the selected events and message detail.
Confirm after checking them. A changed link or settings version requires a new review.

Choose which record changes to share. **Brief** and **Full** control how much context
an allowed message carries. Full summaries keep the relevant decisions, conditions and open
questions from long text. They are limited summaries rather than a transcript. Source content that is partial or too large stays pending with a reason; it is not cut short.

Automatic sharing requires a complete audience whose members can read the record and the shared
content. An unknown or outside audience stops posting. Check the stated reason, fix access
or membership, then review delivery again. Private exchanges do not become a channel feed.

## Read the result in History

A generated summary has the provider's mark and an AI label. Its details say how much talk it
covers. Use **Check source access** before opening the original thread. Source access is checked for
your connected account, even when you may read the retained CRM summary.

Use **Channel recap source** to narrow the full available history to Dispact, Slack or Teams,
including older pages. Summaries show the recorded channel name, update time and references to other
available records they describe.

A source change can update the same generated entry. A human edit is kept and marked for review when the source changes.
Archiving a summary does not restore it on the next change. Confirmed source deletion can remove derived content.
A missing page or a provider outage alone does not prove deletion.

Company History can show allowed work from its deals and projects with the original record source.
It does not grant access to those records or send those entries back to another channel.

A source may request a change to a commitment. Margince can put a question with a source quote in the record owner’s Worklist. It keeps the accepted date. The proposed follow-up has no due date. Accepting it does not accept the requested date. A changed or removed source requires a new review.

## Hand work to delivery

**Start delivery** opens a handoff review with the current room, the new channel's audience,
open promises, dates and any information your permissions leave out. A missing section does not mean
there is no open work.

An existing project channel is kept. If the project has none, the review says which deal room
will continue. To choose a different delivery room, link it to the project in that review before
confirming. The channel keeps its own save, send and detail settings.

## Stop sync and check send results

**Unlink** stops reads, posts and queued work for that link. **Sync ended** retains the end
time and the coverage and delivery times that were recorded. Allowed earlier History remains under
its existing visibility and retention rules. It makes no claim about provider changes after the
last coverage time.

Unlinking does not erase messages already delivered to the provider. Administrators can see
confirmed rewrites from copies that still need a check and deliveries that need checking in the channel.
A confirmed rewrite does not prove that every other copy was removed.

In **Manage**, **Review delivery status** lets an authorized colleague stop retrying selected
sends without a result. It does not mark them delivered or remove a provider copy. An overview card without a result stays paused until a separate confirmation requests a new card.

For private conversations and their own delivery result, use [Messenger](messenger.md).
