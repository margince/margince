<!-- prose:plain -->
<!-- describes: frontend/src/components/messenger frontend/src/features/settings/messagingproviders.tsx backend/internal/modules/chatlinks/messenger.go backend/internal/modules/chatlinks/messengerdelivery.go backend/internal/modules/chatlinks/messengersend.go backend/internal/modules/chatlinks/mergedealrefs.go backend/internal/modules/chatlinks/mergecompanyrefs.go -->
# Messenger on a record

Open **Messenger** beside Email on a company, deal or project. The small window keeps the record
in view while you write. Dispact, Slack and Teams use the same layout, with their own provider marks.

For setup and limits by app, read [What you can do with messaging apps](messaging-apps.md).

## Connect your account

Under **Settings**, **Integrations**, connect your own provider account. This authorizes actions as
you. A shared channel installation is separate and does not replace your personal access.
If your provider access is revoked, reconnect before reopening the conversation.

## Choose recipients and send

Choose the assigned record team, a colleague or an available conversation. Review the sender,
workspace and complete recipient list before sending. Unresolved colleagues stay visible as missing
recipients; Margince does not send to a smaller group. A provider's group limit is shown
before a group is opened.

Choose the relevant thread when replying. Drafts belong to the selected account, record,
conversation and thread. Changing the recipient or provider keeps that context separate.

The window loads replies from your authorized conversation even when the record has no linked
channel or channel sync is off. More messages can be loaded on request. A provider pause keeps
your draft and tells you to wait.

## Check delivery

A confirmed provider receipt marks the message accepted. When the result is unknown, first check
the conversation and use the receipt check. The check reads the app's saved attempt record; it
does not search the provider for a lost receipt.

When companies or deals merge, saved delivery results follow the record you keep. If both records
already opened a group with the same colleagues, that record keeps its existing group. A send with
no confirmed result still needs your review.

A send without a result is not retried automatically. A separate confirmation can open the draft for
a new human attempt, which could duplicate a message that already arrived. Review the conversation again before sending.

## Keep a business outcome

Private conversation bodies are fetched for the window rather than copied into CRM History.
**Share outcome** opens the existing note editor. Review and edit the business result, then save it
as a note. The editor says when that note may also reach a linked work channel.

Use [Team chat channels](chat-channels.md) to choose which channel updates to save and which CRM updates to send.
Sending a quick message does not enable either setting.
