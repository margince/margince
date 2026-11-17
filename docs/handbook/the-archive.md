<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/archivepurpose.tsx frontend/src/features/settings/archivereview.tsx frontend/src/features/privacy/confirmprivacyparts.tsx backend/internal/modules/privacy/archivereview.go backend/internal/modules/consent/controllerparticulars.go -->
# The archive and its purpose

The rule for **All captured activity** moves mail, meetings and calls into an archive with restricted access after 3 years. Margince never deletes them only because they are old. The archive keeps them for a purpose. The privacy page a contact opens says which: "moved to an archive with restricted access to answer legal claims and keep business records". It also says they are deleted when they are no longer needed for this purpose, when the contact asks, or when their record is deleted.

## How do I state why the archive keeps data?

Open **Settings → Privacy and retention → Retention**. In **Archive purpose**, write the words that finish the sentence "moved to an archive with restricted access to …". Where the law asks for another language, such as Vietnamese, the card has a field for that language too. A contact reading the page in that language sees those words.

Leave a field empty and the page uses the default: "answer legal claims and keep business records", in the reader's language. Save with the page's save bar. The words are part of the company details in mail, so the same users may change them.

## How do I delete archived data that no longer serves its purpose?

**Archive review**, on the same tab, lists archived mail, meetings and calls, oldest first. Narrow it with **Occurred before**, **Contact** and **Company**. A contact or company you may not open shows without its name.

Click **Delete … shown records** and confirm. Margince erases the content of each record. Who it was with and when stays, as with the **Erase** retention action. Nothing on this screen runs by itself: a record is deleted only when you confirm.

Some records stay, and the list says why before you delete:

- **Kept: legal hold**: the record is linked to a contact, company, deal, lead or project on legal hold.
- **Kept: opt-out record**: the record is the receipt of a refusal, and the law still keeps it.
- **Kept by law**: business correspondence still inside its statutory retention period.

After a deletion, the screen counts what was deleted and what was kept, by reason. One request deletes at most 100 records, and the button sends larger sets in turn. Every deletion is in the audit trail as an erase, with the archive review as its cause. Only an admin or ops user who may change retention sees the review and may delete.
