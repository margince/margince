<!-- prose:plain -->
<!-- describes: frontend/src/features/capture/connectors.business.tsx frontend/src/features/capture/connectors.businessdialogs.tsx backend/internal/modules/contacts/channelphone.go backend/internal/modules/capture/channelwhatsapp.go backend/internal/modules/capture/channelzalo.go backend/internal/modules/capture/chatseat.go backend/internal/modules/capture/chatmessage.go backend/internal/modules/capture/chatchange.go -->
# Connecting WhatsApp and Zalo

A company can connect one WhatsApp Business number or one Zalo account. What customers write to it shows in the
**History** of the customer. So does what your team writes back in the WhatsApp Business app or in Zalo.

An administrator opens **Settings → Integrations → Messaging accounts**. Each provider has its own panel.
Press **Connect account**, enter the account credentials and choose the team that reads its messages.
Credentials stay hidden after connecting. Both accounts start **Held**.

For WhatsApp, enter the WhatsApp Business Account ID, phone number ID, display number, access token and app secret.
After connecting, copy the **Webhook verification token** into the provider's webhook settings before closing the dialog.
It appears only once.

For Zalo, enter the OA ID, app ID, account name, access token, refresh token, app secret and OA secret key. An account whose access expired says **Needs reconnection**. Disconnect it and connect it again
with new credentials.

The setup steps cover provider webhook events in [connect-whatsapp.md](../how-to/connect-whatsapp.md)
and [connect-zalo-oa.md](../how-to/connect-zalo-oa.md).

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

### Where is the customer's WhatsApp number?
A phone-based WhatsApp account fills the customer's **Phone** field and gives the number for its **WhatsApp** chat link. Existing numbers stay, and a
number already archived on the contact stays archived. A business-scoped user ID is not a phone number and fills
no phone field. The background task also fills older accounts whose retained provider originals name the number.
A number given only by a colleague in an answer does not prove a WhatsApp phone. Phone numbers follow the
contact's visibility; holding a message limits its readers, not the contact's phone fields.

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
