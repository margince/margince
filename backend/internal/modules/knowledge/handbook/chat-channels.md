<!-- prose:plain -->
<!-- describes: frontend/src/components/channelpanel.tsx frontend/src/components/channellinkdialog.tsx frontend/src/features/capture/connectors.telegram.tsx backend/internal/modules/chatlinks -->
# Team chat channels

A deal or a project in Margince can have its own private channel in your team chat, Dispact. Margince
keeps a card at the top of the channel with the record's name, stage, close date and owner. It also
posts what happens on the record, so the team sees it where they already talk.

A deal and its project share one channel.

### How do I connect the team chat?
An admin connects the Dispact bot once for everyone.
1. Open **Settings**, then **Integrations**, then **Chat bots**.
2. Click **Connect Dispact bot**.
3. Enter the **Chat address**, such as `https://chat.example.com`, and the app token, which starts with
   `gwapp_`.

### How do I give a deal a channel?
Open the deal. The **Team chat** card is on its overview.
- Click **Create a channel** to make a new private channel named after the deal. You and the deal's
  owner are invited.
- Click **Link an existing channel** to use a private channel you already have. Enter the **Channel name
  or link**. The bot must already be in that channel, and the channel must be private.

A project's **Team chat** card works the same way.

### What does Margince post?
**What Margince posts** sets how much reaches the channel:
- **Everything**: every change to the record.
- **Important only**: stage and owner changes, the close date, offers, contracts, and shared notes,
  meetings and mail. This is the default.
- **Milestones only**: stage, offer, contract and phase steps.
- **Silent (card only)**: nothing is posted, and the card stays current.

Margince waits two minutes, then sends the changes from that time together.

### Will a private email reach the channel?
No. A note, meeting or email reaches the channel only when everyone in your workspace may read it. If
you make it private after it was posted, Margince removes it from that post.

### Why does the card show no amount?
Some members may not see deal amounts, and everyone in the channel reads the card.

### What does "Channel paused" mean?
The channel was archived or deleted in the chat, or the bot lost access to it. Nothing is posted.
Click **Unlink**, then give the record a new channel.

### How do I stop it?
Open the card's menu and click **Unlink**. Margince stops posting. The channel and its messages stay
in the chat.
