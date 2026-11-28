<!-- prose:plain -->
<!-- describes: frontend/src/components/messenger frontend/src/components/channelpanel.tsx frontend/src/components/channelsyncsettings.tsx frontend/src/features/settings/messagingproviders.tsx backend/internal/modules/chatlinks backend/internal/compose/chatsync -->
# What you can do with messaging apps

Your workspace selects one team messaging service: Dispact, Slack or Microsoft Teams. You can write messages from a record and connect a work channel to that record.
Each app has limits. Connecting an app does not turn on every feature or give everyone access to its conversations.

## Send a message or keep the record up to date?

| What you want | Where to go | What happens |
|---|---|---|
| Ask a colleague a question | Messenger beside Email | A small window opens over the record. You choose the colleagues or conversation and send the message yourself. |
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
| Link an existing work channel to a company, deal or project | After setup | After setup | **Standard**, private or shared channels with checked app access |
| Save clear channel summaries to History | Needs the updated Dispact connection | Needs a supported Slack connection; see below | After setup; private and shared channels need the app added there too |
| Send automatic CRM updates to the channel | After audience review | Private internal channels after audience review | Supported internal channels after audience review |
| Create a work channel from Margince | Available where setup allows it | Available where setup allows it | Cannot do this; create the channel in Teams, then link it |
| Add colleagues to the linked channel from Margince | Available where setup allows it | Available where setup allows it | Cannot do this; manage members in Teams |
| Link a Teams shared channel | — | — | After the app is added there and access is checked |

A normal Teams channel is open to the members of its team. Teams calls this a “standard” channel. A private channel is restricted to selected members of that team.
A shared channel can include members from other teams or companies. Margince checks its current audience before sending CRM information.

Some limits come from the messaging app. Others are features Margince does not yet offer, such as creating Teams channels.
You can use the app's other features when an action is blocked.

## What setup do I need?

Your administrator chooses the service in **Settings**, **Integrations**, **Messaging accounts**, then connects it for shared channel work. Changing the service disconnects the old shared connection and personal accounts, and ends its channel links. History stays available. Link records to channels in the new service before enabling sync again. Colleagues connect their own accounts to the new service.

You connect your own account to use Messenger as yourself.
A shared connection does not let Margince send personal messages as you.


Ask your administrator to check the shared connection before your team turns on automatic summaries or posts.
Check the sender and recipient list before sending a personal message. Only colleagues found in the messaging app can be added.
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
Automatic summaries in Margince need either an app built for your own company or an app approved for the Slack app store.
A commercial app without that approval can have lower limits. The current Margince connection cannot give automatic complete channel summaries under those limits.

Personal Messenger also follows the reading limits set by Slack. A lower-limit connection can read a small page and then must wait. It cannot prepare an immediate outcome draft; you can still write and save the outcome yourself. Allowed CRM posts have separate access checks.

Your administrator records the way the app is shared in **Messaging accounts**, even when your team uses only personal Messenger. This is the way the app is shared, rather than your Slack subscription. Until confirmed, personal reading uses the lower limits.

Already loaded messages stay visible while Slack asks Margince to wait.
Ask your administrator whether the Slack connection supports automatic summaries before you use them.

Existing private-channel bot connections can keep working. Connect the app again with the updated permissions to add public-channel reading.


The shared Slack bot must be added to the linked channel. It reads replies using its own permissions; it does not depend on the colleague who connected it staying in that channel.
Public Slack channels can be linked for reading. Automatic CRM posts remain limited to private internal channels.

## What is different about Teams?

For Teams automatic channel work, the app must be added and allowed to access the selected team.
Private and shared channels need the app added to each channel too. Adding it to the team alone does not grant private channel access.
The connection checks whether the app can read the selected channel. An administrator can approve several teams in the same company.

Automatic posts need an internal group whose members may read the CRM record. A private or shared channel needs checked app access and the same audience review. A channel with outside members cannot receive automatic CRM posts.
Create channels and manage their members in Teams. Then use Margince to link a supported existing channel.

To stop access to one approved team, remove the app from that team in Teams. Other approved teams can keep working. Use **Disconnect service** in Margince to stop the whole service.
After upgrading an older connection, relink a private or shared channel to check its current posting access before enabling CRM posts.

You can still use personal Messenger for conversations your connected Teams account may open. Existing chats appear with channels. After reconnecting an account, choose a conversation again so Margince can check the current identity and recipients.
Private chat messages and a private Teams channel are different things. Each has its own access check.

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
