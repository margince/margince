<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/retainedsignature.tsx backend/internal/modules/contacts/retainedsignature.go frontend/src/features/settings/settingssignaturerow.tsx frontend/src/features/mail/composesignoff.tsx backend/internal/modules/contacts/emailsignature.go backend/internal/modules/activities/signature.go backend/internal/modules/activities/signaturetemplate.go frontend/src/features/settings/legalentityeditor.tsx frontend/src/features/settings/settingssignatureeditor.tsx backend/internal/modules/activities/htmlsanitize.go backend/internal/modules/activities/signaturepolicy.go backend/internal/compose/signaturepreviewimages.go frontend/src/features/automations/approvalevidence.tsx frontend/src/features/settings/gmailsignatureimport.tsx backend/internal/compose/gmailsignature/gmailsignature.go -->
# Your email signature

Open **Settings → Account → Edit signature**. Choose **Plain text** to write a
sign-off, or **HTML** to paste a formatted signature. Check the preview and choose
**Save**. The composer shows the same sign-off before you send.

To use the signature you already have in Gmail, choose **Use my Gmail signature**
in the same dialog. Margince reads the signature of the address Gmail sends from,
puts it in the **HTML** field and fills the plain text with its words. Nothing is
saved until you choose **Save**. Without a connected Gmail mailbox, or when
Margince may not read your Gmail settings, the dialog says to connect or
**Reconnect** under **Settings → Connections**.

Margince uses your own HTML first, then the company template, then your saved
plain text. Clear the HTML field, or choose **Plain text** and save, to use the
company template again. With none of these set, a send ends with a short greeting
and your name.

An Admin can assign you to one of the companies on the **Legal entities** card.
If that company has its own template, it takes the place of the company template.
The logo is then that company's, and one without a logo shows none. The template
can also show that company's imprint.

The greeting follows the message's language. When the message is too short to
tell, Margince uses the installation's language, then English.

Personal HTML can be up to 10,000 characters, counted as you typed or pasted
it. Past 8,000 characters the field shows a count, and it turns red past the
limit. **Save** then tells you the signature is too long.

The HTML preview shows what a send keeps. A signature pasted from your mail
client keeps its tables, layout, colours and logo. Images from `https` addresses
are kept at the size you set. Images from other addresses are removed, and so
are scripts, forms and embedded pages.

The preview shows each image from a copy Margince fetches. If an image cannot be
fetched, the preview shows a box of its size with its name. A list of approvals
shows each image as that box. On an approval, choose **Show the images** under
the signature to see them. The sent mail still links the image.

To show the logo of the company you work for without hosting it yourself, write
`<img src="cid:signature-logo@margince">`. Margince embeds that logo in the
mail itself.

If no text remains, the usual fallback signs the mail.
The settings row names the format you saved; check the preview for the result.
Fields such as `{name}` stay as typed in personal HTML. A plain message
also has a formatted version when the signature uses HTML.

The sign-off comes above any required notice and the unsubscribe footer. An email
an agent sends carries no sign-off. AI drafts never add one.

## Review a retained signature for an inactive member

A **Manual privacy review needed** notification opens the retained signature it
names. A privacy controller can read its plain text, HTML, title and phone, then
edit them and choose the page's **Save**. Opening it keeps the original text.
The preview shows the saved HTML as a send keeps it, with scripts and images
that are not from `https` addresses removed.

A controller needs a full seat, an Admin or Ops role, and access to read and erase
all contacts without hidden fields. A read-only seat cannot review it. The owner
must not be an active human member; their signature becomes private again
if they become active. Your own signature stays separate from this review.

If another controller changes a field you did not edit, Margince keeps their change when saving
yours. If you both changed the same field, your draft stays. Review **Latest saved version**, then
choose **Use latest version** or **Keep my draft** before saving again. Keeping your draft replaces
the displayed fields with your values. Saving a retained signature does not restore an archived signature.
