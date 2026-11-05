<!-- prose:plain -->
<!-- describes: frontend/src/components/conversationsuggestions.tsx backend/internal/modules/capture/partyadmission.go backend/internal/modules/capture/conversationsuggestions.go -->
# Other recipients on a captured conversation

When your connected mailbox confirms that you sent a message, Margince checks each
outside address in **To** and **Cc**, not just the first recipient. An eligible
business address can become a contact. A **Bcc** recipient does not become a contact this
way. A message waiting to send in Margince does not count as sent.

For a calendar meeting, you must have sent or accepted the invitation.
Eligible outside guests and whoever sent the invitation can become contacts. A declined guest,
a canceled meeting or a meeting marked private does not qualify. Your rules for
skipped addresses, internal or erased addresses and automated senders still apply.
An existing lead stays a lead until you choose to qualify it.

Other recipients named in mail you received may appear on a contact or lead under
**On this conversation, not in Margince yet**. Choose **Add as contact** to add one
through the usual contact form. This list uses only messages you may read and does
not add anyone by itself. It starts with recent messages; **Older messages may name
other recipients** means the list reached its reading limit.
