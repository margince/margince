<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/retainedsignature.tsx backend/internal/modules/contacts/retainedsignature.go frontend/src/features/settings/settingssignaturerow.tsx frontend/src/features/mail/composesignoff.tsx backend/internal/modules/contacts/emailsignature.go backend/internal/modules/activities/signature.go backend/internal/modules/activities/signaturetemplate.go frontend/src/features/settings/legalentityeditor.tsx -->
# Your email signature

Open **Settings → Account → Edit signature**. Choose **Plain text** to write a
sign-off, or **HTML** to paste a formatted signature. Check the preview and choose
**Save**. The composer shows the same sign-off before you send.

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

The HTML preview shows what a send keeps. Images from websites and scripts are
removed. Only the built-in logo of the company you work for can be embedded; other image attachments
are removed too.

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
The preview shows the saved HTML with images from other sites and scripts removed.

A controller needs a full seat, an Admin or Ops role, and access to read and erase
all contacts without hidden fields. A read-only seat cannot review it. The owner
must not be an active human member; their signature becomes private again
if they become active. Your own signature stays separate from this review.

If another controller changes a field you did not edit, Margince keeps their change when saving
yours. If you both changed the same field, your draft stays. Review **Latest saved version**, then
choose **Use latest version** or **Keep my draft** before saving again. Keeping your draft replaces
the displayed fields with your values. Saving a retained signature does not restore an archived signature.
