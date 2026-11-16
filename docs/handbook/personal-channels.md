<!-- prose:plain -->
<!-- describes: frontend/src/features/capture/personal-channels.tsx backend/internal/modules/personalchannels backend/internal/compose/personalchannelwire -->
# Personal channels: bring your own chats into Margince

Your customers write to you on WhatsApp, Zalo and other chat apps that Margince cannot read. **Personal channels** let you bring those chats in yourself. You choose what is sent; each message lands on the contact's timeline, and you can reply from Margince.

### How do I bring a WhatsApp or Zalo chat into Margince?
To bring a chat into Margince, turn on personal channels and ask your AI assistant to send it in.
1. Open **Settings → Connections** and choose **Turn on** in **Personal channels**.
2. Open **Connect your AI tool** in **Settings → AI assistants**. On **Authorize access**, tick **Personal channels**.
3. Ask your assistant, for example: "Send this chat with An to Margince."
Any AI assistant that connects over MCP works: Claude, ChatGPT, Gemini, Copilot and others.
Also called: WhatsApp in Margince, Zalo in Margince, log a chat, import a chat.

### Can my AI assistant send files from a chat?
Your AI assistant can send in a file a chat carried, such as a photo or a PDF. It lands on the activity like a file on an email. Through an AI assistant a file can be about 6 MB; a client of your own can send up to 25 MB.

### Who can see the chats I send in?
The chats you send in follow your workspace's sharing setting, the same as captured mail. Someone who first writes to you on a chat becomes a contact the whole workspace can see. Send in only what belongs in the CRM; Margince never reads your chat apps by itself.

### How do I reply to a chat from Margince?
To reply to a chat, write the reply on the contact's timeline as you would any message. Margince keeps it until your client picks it up and sends it on your chat app. In **Settings → Connections**, **Replies** shows how many wait and when your client was last seen.
Only you can reply on a chat you sent in, because the reply goes out from your own chat app. A reply nobody picks up within 7 days is marked **Did not arrive**.

### How do I stop sending chats in?
To stop, choose **Turn off** in **Personal channels** in **Settings → Connections**. No assistant or client can then send chats in or pick replies up. Replies already waiting stay and still expire after 7 days.

### Can I use my own program instead of an AI assistant?
You can use a program of your own with a passport that has the **Personal channels** permission. See [Connect a chat client with the Personal channels API](personal-channels-api.md).
