<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/webhooks.tsx frontend/src/features/settings/webhookdrawer.tsx frontend/src/features/settings/webhooks.secret.tsx backend/internal/modules/webhooks -->
# Sending events to other tools

A webhook sends a message to another tool when something happens in Margince: a deal is won, a
contact is added, an offer is sent. Use it to post to a team chat channel, or to feed another
system. Admin and Ops set webhooks up in **Settings → Integrations**, under **Webhooks**.

### How do I post to Slack or Teams when a deal moves?
To post to Slack or Microsoft Teams when a deal moves, open **Settings → Integrations** and choose **New subscription** in **Webhooks**.
1. In Slack or Teams, create an incoming webhook for the channel and copy its link.
2. Paste the link in **Target URL**. It must start with `https://`.
3. Choose **Add events**, tick **Deal moved to another stage**, then **Done**.
4. Set **Body format** to **Custom template**, and in **Start from** pick **Slack** or **Microsoft Teams**.
5. Check **Preview**, then choose **Create subscription**.
Also called: deal won notification, Slack alert, chat message, Teams message.

### How do I change what a message says?
To change a message body in Margince, edit the subscription and set **Body format** to **Custom template**, then write the message in the template. A template fills `{{ entity.name }}` with the name of a deal, company or project, `{{ entity.owner_name }}` with its owner, and `{{ entity.url }}` with a link to its page. For a contact or lead, both names are empty and the message carries the link. To show a deal's amount when it moves, write `{{ data.amount_minor_at_change | money data.currency_at_change }}`, which reads as `56,830.20 USD`. **Insert variable** lists every field the chosen event has, and the ⓘ beside an event in **Add events** shows them too.
Also called: customise the webhook body, message text, payload, fields, deal name in the message, deal owner, deal amount.

### Does a sent message keep a contact's name?
Yes, if the message carries it. The event's own fields can name a contact: a new contact's event carries `{{ data.full_name }}`. Margince keeps each sent message, even after the contact is erased. Leave a name out of the template when the link is enough.
Also called: privacy, erasure, personal data in webhooks.

### Can I get a message only when a deal is won?
No. A subscription to **Deal moved to another stage** sends one message for each move, won and lost included, and a template cannot skip a move. Put the new stage in the message (`{{ data.to_stage_name }}`, `{{ data.to_status }}`) so a reader sees which one it was.
Also called: message for every stage, too many messages, only won deals, filter events.

### How do I connect Margince to Zapier or another automation tool?
To connect Margince to Zapier or another automation tool, use a webhook: there is no separate app. In that tool, start an automation with its webhook trigger and copy the link it gives. In Zapier, that trigger is **Catch Hook** in **Webhooks by Zapier**. In Margince, create a subscription in **Settings → Integrations** with that link as **Target URL**, and pick the events. Keep **Body format** on **Standard envelope**, so the tool gets every field.
Also called: Zapier, Make, automation tool, integrate with another app.

### How do I send a token or key with each message?
To send a token with each message in Margince, edit the subscription and choose **Add Authorization** or **Add header** under **Custom headers**. Type the header name and its value, then save. The value is locked away when saved and never shown again: the row reads **Value set**, and **Replace** sets a new one. Names that Margince sets itself, such as `Content-Type` and the signature headers, are refused.
Also called: API key, access token, secret header.

### How do I check that messages arrive, or send a failed one again?
To check deliveries in Margince, open **Settings → Integrations** and choose **View deliveries** on the subscription. Each row shows **Delivered**, **Retrying** or **Dead-lettered**. A **Dead-lettered** row stopped after its last try; choose **Replay** on it to send it again.
Also called: webhook log, failed webhook, resend, retry, delivery history.

### How do I stop a webhook?
To stop a webhook for a while, choose **Edit** and set **State** to **Paused**; nothing is sent until you set it back to **Active**. To stop it for good, choose **Archive** in the row's **⋯** menu.
Also called: turn off webhook, disable notifications, delete subscription.

### What is the signing secret?
The signing secret is shown once, when you create a subscription. A system that receives your messages can use it to check that a message came from Margince. Copy it then, because you cannot see it again. **Rotate secret** in the **⋯** menu makes a new one.
Also called: webhook secret, signing key.

### How do I check that a message came from Margince?
To check a message, the receiving system checks its `webhook-signature` header with the signing secret. Margince signs each message by the common webhook signing rules (HMAC-SHA256), so a webhook signing library for your language can do the check. The signature covers the `webhook-id` and `webhook-timestamp` headers and the body. Refuse a message whose signature does not match or whose time is old.
Also called: webhook signature, check a signature, webhook security.
