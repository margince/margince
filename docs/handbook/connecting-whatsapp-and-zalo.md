<!-- prose:plain -->
<!-- describes: backend/internal/modules/capture/channelwhatsapp.go backend/internal/modules/capture/channelzalo.go backend/internal/modules/capture/chatseat.go backend/internal/modules/capture/chatmessage.go backend/internal/modules/capture/chatchange.go -->
# Connecting WhatsApp and Zalo

A company can connect one WhatsApp Business number or one Zalo account. What customers write to it shows in the
**History** of the customer. So does what your team writes back in the WhatsApp Business app or in Zalo.

There is no settings card for these two yet. An administrator connects them with the steps in
[connect-whatsapp.md](../how-to/connect-whatsapp.md) and [connect-zalo-oa.md](../how-to/connect-zalo-oa.md).

### Who can read the messages?
At first, only the team the administrator chose when connecting. That is **Held**. A member outside that team does not
see the messages.

An administrator can change it to **Shared**. Then a message follows the sharing of the contact it belongs to, as an
email does. **Shared** needs the company's opt-in. The change applies to messages that come in after it. Messages held
before stay held.
Also called: who sees WhatsApp chats, reader team, held and shared.

### Can I reply from Margince?
No. These chats are read-only in Margince. Reply in the WhatsApp Business app or in Zalo, and the reply shows in
**History** a moment later. If you try to reply from Margince, it says so and sends nothing.
Also called: reply to WhatsApp, reply to Zalo, read-only chat.

### Which messages show in History?
Text from the customer, and text your team types in the app. All of a customer's messages sit in one thread on one
contact, even when the reply is the first thing Margince sees.

A photo, a file or a voice note is kept with the message, up to 25 MB for a file and 50 MB for a message. If a file
is too big or cannot be fetched, the message still shows, with a note that the file was not kept. The words under it stay the
message text. A reaction, a delivery tick and a "seen" mark add nothing.

### What happens when a message is edited or deleted?
When a customer edits a WhatsApp message, the text in **History** changes to the new text. When a message is deleted
on a company number, Margince keeps the copy, because the whole team may delete there. Zalo has no edits.
Also called: edited WhatsApp message, deleted message.

### What if two numbers have the same customer?
Each number or account has its own threads. A customer who writes to two of your numbers has two threads, and
Margince never mixes them.

### What happens when I erase a customer?
Erasing a contact also destroys the stored original of every WhatsApp or Zalo message of that contact. A message that
arrives later from the same customer is not stored.
See [Retention, exports and deletion](retention-exports-and-deletion.md).

### What happens when the connection is removed?
The administrator disconnects the number or account. Margince stops taking messages at once and deletes the stored
keys. Everything already in **History** stays. If the same number is connected again, its threads carry on where they
were.
Also called: disconnect WhatsApp, remove Zalo account.
