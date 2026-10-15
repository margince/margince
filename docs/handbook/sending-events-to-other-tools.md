<!-- prose:plain -->
# Sending events to other tools

A webhook sends a message to another tool when something happens in Margince: a deal is won, a
contact is added, an offer is sent. Use it to post to a team chat channel, or to feed another
system. Admin and Ops set webhooks up in **Settings → Integrations**, under **Webhooks**.

### How do I post a message to a chat channel when a deal moves?
To post to a chat channel in Margince, open **Settings → Integrations** and choose **New subscription** in **Webhooks**. Paste the channel's incoming webhook link and pick the deal event.
1. In your chat tool, create an incoming webhook for the channel and copy its link.
2. Paste the link in **Target URL**. It must start with `https://`.
3. Choose **Add events**, tick **Deal moved to another stage**, then **Done**.
4. Set **Body format** to **Custom template**, and in **Start from** pick **Slack**, **Microsoft Teams** or **Generic JSON**.
5. Open **Preview** to see the message, then choose **Create subscription**.
Also called: deal won notification, notify finance, chat alert, Slack message, Teams message.

### What can a message say?
A message can say what happened and link to the record. A template fills `{{ entity.name }}` with the name of a deal, company or project, and `{{ entity.url }}` with a link to its page. For a contact or lead the message carries the link only, so a person's name never sits in a stored message: open the link to see who it is. **Insert variable** lists every field the chosen event has, and the ⓘ beside an event in **Add events** shows them too.
Also called: message text, payload, fields, variables, deal name in the message.

### Why did my chat channel get a message for every stage, not only won deals?
A subscription to **Deal moved to another stage** sends one message for each move, won and lost included. A template cannot skip a move. Put the new stage in the message (`{{ data.to_stage_name }}`, `{{ data.to_status }}`) so a reader sees which one it was.
Also called: too many messages, only won deals, filter events.

### How do I send a token or key with each message?
To send a token with each message in Margince, edit the subscription and choose **Add Authorization** or **Add header** under **Custom headers**. Type the header name and its value, then save. The value is locked away when saved and never shown again: the row reads **Value set**, and **Replace** sets a new one. Names that Margince sets itself, such as `Content-Type` and the signature headers, are refused.
Also called: API key, bearer token, authorization header, secret header.

### How do I check that messages arrive?
To check deliveries in Margince, open **Settings → Integrations** and choose **View deliveries** on the subscription. Each row shows whether the message was delivered, is waiting to try again, or stopped. A stopped one can be sent again with **Replay**.
Also called: webhook log, failed webhook, resend, retry.

### How do I stop a webhook?
To stop a webhook for a while, choose **Edit** and set **State** to **Paused**; nothing is sent until you set it back to **Active**. To stop it for good, choose **Archive** in the row's **⋯** menu.
Also called: turn off webhook, disable notifications, delete subscription.

### What is the signing secret?
The signing secret is shown once, when you create a subscription. A system that receives your messages can use it to check that a message came from Margince. Copy it then, because you cannot see it again. **Rotate secret** in the **⋯** menu makes a new one.
Also called: webhook secret, signature, verify webhook.
