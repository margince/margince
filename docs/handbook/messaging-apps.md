<!-- prose:plain -->
<!-- describes: frontend/src/components/messenger frontend/src/components/channelpanel.tsx frontend/src/components/channelsyncsettings.tsx frontend/src/features/settings/messagingproviders.tsx backend/internal/modules/chatlinks backend/internal/compose/chatsync -->
# What you can do with messaging apps

Margince connects to Dispact, Slack and Microsoft Teams. You can write messages from a record and connect a work channel to that record.
Each app has limits. Connecting an app does not turn on every feature or give everyone access to its conversations.

## Send a message or keep the record up to date?

| What you want | Where to go | What happens |
|---|---|---|
| Ask a colleague a question | Messenger beside Email | A small window opens over the record. You choose the people or conversation and send the message yourself. |
| Keep key channel decisions on the record | Team chat on Overview | Margince reads the linked channel and saves relevant summaries to History when this setting is on. |
| Tell the channel about changes in Margince | Manage on Team chat | Margince sends allowed record updates when you turn on this setting and approve the audience. |

These actions are separate. Sending a message does not start automatic channel reading or posting.
You can use Messenger without linking a channel. A linked channel can keep working while the Messenger window is closed.

For example, ask a colleague, “Can you confirm the trial date?” in Messenger.
Use a channel link when the team wants trial decisions saved on the deal over time.

## What can I do with each app?

“After setup” means the feature needs a working connection and the required access. It does not mean every channel is allowed.

| What you want | Dispact | Slack | Teams |
|---|---|---|---|
| Write to a colleague or group from Messenger | After setup | After setup | After setup |
| Open replies in Messenger | After setup | After setup | After setup |
| Link an existing work channel to a company, deal or project | After setup | After setup | A normal Teams channel; private channel access needs a separate check |
| Save clear channel summaries to History | Needs the updated Dispact connection | Needs a supported Slack connection; see below | A normal Teams channel after setup; private channels need the app added there too |
| Send automatic CRM updates to the channel | After audience review | Private internal channels after audience review | A normal Teams channel after audience review |
| Create a work channel from Margince | Available where setup allows it | Available where setup allows it | Cannot do this; create the channel in Teams, then link it |
| Add people to the linked channel from Margince | Available where setup allows it | Available where setup allows it | Cannot do this; manage people in Teams |
| Link a Teams shared channel | — | — | Cannot do this |

A normal Teams channel is open to the members of its team. Teams calls this a “standard” channel. A private channel is restricted to selected people within that team.
A shared channel is a separate Teams channel type. It is outside the current Margince channel-link support.

Some limits come from the messaging app. Others are features Margince does not yet offer, such as creating Teams channels.
You can use the app's other features when an action is blocked.

## What setup do I need?

Your administrator connects the messaging app for shared channel work. You connect your own account to use Messenger as yourself.
A shared connection does not let Margince send personal messages as you.

Ask your administrator to check the shared connection before your team turns on automatic summaries or posts.
Check the sender and recipient list before sending a personal message. Only people found in the messaging app can be added.
A company contact in Margince does not get a chat account or join a conversation because the company has a channel link.

### How do I open messaging settings?
<!-- opens: settings/integrations -->
To open messaging settings, choose **Settings**, then **Integrations**, then **Messaging accounts**.
Connect your personal account there. Ask an administrator to complete shared setup when it is missing.
Also called: connect chat, messaging setup.

## What is different about Dispact?

Dispact needs the updated connection to give the channel information required for complete summaries and audience checks.
Your administrator must check that the required Dispact update is running and that the connection has access.
Older connections may still offer their existing features. That does not prove the new features are available.

The Dispact update only applies to Dispact. Slack and Teams have their own ways to read channels and send messages.

## What is different about Slack?

Slack limits how quickly an app can read old messages and thread replies.
Automatic summaries in Margince need either an app built for your own organisation or an app approved for the Slack app store.
A commercial app without that approval can have lower limits. The current Margince connection cannot give automatic complete channel summaries under those limits.

That restriction applies to automatic channel summaries. Personal Messenger and allowed CRM posts have separate access checks.
Ask your administrator whether the Slack connection supports automatic summaries before you use them.

The shared Slack connection also needs permission to read the linked channel's replies.
If the person who connected it loses channel access, incoming summaries pause. Another colleague's personal Messenger connection does not fix that shared connection.

## What is different about Teams?

For Teams automatic channel work, the app must be added and allowed to access the selected team.
A private channel needs the app added to that channel too. Adding it to the team alone does not grant private channel access.
Private channel reading needs a check with the connected app before you use it.

Margince cannot send automatic CRM posts to private Teams channels. Shared Teams channels also cannot be linked.
Create channels and manage their members in Teams. Then use Margince to link a supported existing channel.

You can still use personal Messenger for conversations your connected Teams account may open.
Private chat messages and a private Teams channel are different things. The channel-posting limit does not turn off all private conversations.

## Will I get a clear summary without opening the chat?

Margince writes the summary. It keeps the decision, conditions and open questions clear enough to read from History.

For example, a summary can say: “The buyer wants a trial after security approval. Wednesday was requested; Friday remains agreed. The owner must confirm access.”
It keeps a requested date separate from an agreed date. A source link is there when you need the full conversation.

**Full** adds context to allowed CRM posts. It does not grant access to private information or turn a summary into a transcript.
Missing access, unread pages or source size limits can stop a summary. Margince keeps that work pending with a reason instead of claiming complete coverage.

## What will not happen automatically?

Your private Messenger conversations are not copied into record History. Use **Share outcome** when you want to save a reviewed business result.
A channel link does not import every old conversation. Turning reading back on starts with new channel activity rather than filling the gap.

Changing a deal's team does not guarantee the chat's members change with it. Review the audience before sharing record information.
An unknown or outside audience can stop automatic posting, even when the provider can technically send the message.

**Unlink** stops future sync for that link. It does not delete messages already sent to the messaging app.
A missing source does not prove someone deleted it. Margince needs confirmation before removing content derived from a deleted message.

For the record controls, read [Messenger on a record](messenger.md) and [Team chat channels](chat-channels.md).
