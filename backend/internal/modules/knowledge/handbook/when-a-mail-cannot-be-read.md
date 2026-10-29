<!-- prose:plain -->
<!-- describes: frontend/src/design-system/emailthread.tsx backend/internal/shared/kernel/provenance/mailbody.go backend/internal/modules/capture/mailmap/body.go backend/internal/modules/capture/mailmap/partwalk.go backend/internal/modules/activities/mailbodycompleteness.go backend/internal/modules/activities/mailbodyrestore.go backend/internal/modules/activities/commitmentadmission.go -->
# When a mail cannot be read

Margince stores every captured mail with the text it could read. That is not
always all of it. The email then says so above the text, and Margince does not
act on the mail by itself.

## What the email says

- **The text could not be read.** The text is in a form Margince cannot read,
  such as an Outlook `winmail.dat` file. Only the From and To lines are stored.
- **Part of the text could not be read.** Only the first part of the text could
  be read, or the mail was too long to store whole.
- **The text is the forwarded mail.** A forward that adds no words of its own
  shows the text of the forwarded mail. That text is not from the sender.
- **Email text not verified.** The mail was stored before Margince
  checked this. Margince checks it against the stored copy of the mail, and
  restores text that an older version could not read. A mail with no stored
  copy stays unchecked. The notice says that Margince cannot find requests or
  commitments in it. Read the text and add a task if you need to follow up.

If text is missing, open the mail in your mail program to read all of it.

## What Margince does not do with it

Margince does not find promises or requests in such a mail, and does not count
it as an answer to one. It does not read events in a conversation until every
mail on it was checked and read in full.
