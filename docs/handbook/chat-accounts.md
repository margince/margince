<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/chatidentities.tsx backend/internal/modules/identity/chatidentity.go backend/internal/modules/capture/chatforward.go backend/internal/modules/capture/chatasker.go backend/internal/modules/capture/chatclarifyreply.go -->
# Your chat accounts

You can confirm your own WhatsApp, Telegram and Zalo accounts in Margince. Then a customer message you pass on from your phone to a company chat account is filed for that customer. It is never filed as a message from you.

### How do I confirm my chat account?
To confirm your own chat account in Margince, open **Settings**, then **Account**. The rows are on the **Profile** card.
1. Find the row for your network: WhatsApp, Telegram or Zalo. Click **Confirm with a code** on it.
2. Margince shows a short code. It works for 10 minutes.
3. Send that code as a message from your own account to a company account on the same network.

Margince files nothing for that message. The row then shows your account with most of its number hidden. Click **Remove** to take it off.

Also called: link my WhatsApp, link my Telegram.

### What happens when I pass on a customer's message?
That depends on what the network says about who wrote it.
- When Telegram names the writer, the message goes on that customer's contact. It carries the time the customer wrote it.
- When Telegram hides the writer, or you use WhatsApp or Zalo, the network does not say who wrote it. Margince keeps the message for you alone, makes no contact and asks you who wrote it (see below). It is deleted after 7 days if nobody files it.

If your account is not confirmed, the message is filed as yours, and you appear as a contact.

### How does Margince ask who wrote a message?
Margince asks in the same chat, from the same company account.
1. You pass on one or more customer messages. Messages you pass on within 2 minutes of each other count as one set.
2. Two minutes after the last one, the company account writes to you: "Who wrote this? Reply with their contact card or phone number." On Telegram and Zalo it asks for the contact card only. Those networks know a customer by account, not by phone number.
3. Reply to that question with the customer's contact card or, on WhatsApp, with their phone number.
4. Margince finds the customer's contact, or creates it, and files every message of the set on it.
5. You are recorded as the one who passed them on.

The question, your reply and any copy of them that the network sends back are not customer messages. They never show in a customer's history and never create a contact. Margince deletes the stored copies once it has used them.

If you have passed on several sets and answer without replying to a question, Margince files nothing. It sends you one message that lists the sets, each with the time and the start of its first message. Then reply to the question you mean. If only one set is waiting, your reply goes to that one.

A confirmed colleague's own messages to a company account are never filed as customer messages, and they never make a contact. This covers a "thanks" and a card that names no customer. It covers anything else that is not a code, a forward or an answer. Margince deletes the stored copy. If a set is waiting for you to say who wrote it, it sends you one short reminder to reply with the customer's contact card. It sends at most one reminder for each set.

If you file the messages by hand in Margince first, the set is closed. A later reply to its question is used up and changes nothing.

WhatsApp lets a company account write to you for 24 hours after you last wrote to it, and Zalo for 48 hours. After that Margince cannot ask, and the messages wait until they expire. If the network refuses the question for another reason, such as a limit, Margince tries again after a pause. It never sends the same question twice when it does not know whether the first one arrived.

The question is in your language in Margince: English, German or Vietnamese.

### Can someone else confirm my account?
No. One account belongs to one member. A member has one Zalo account, and it counts for one Zalo company account.
