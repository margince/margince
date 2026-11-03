<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/settingssignaturerow.tsx frontend/src/features/mail/composesignoff.tsx backend/internal/modules/contacts/emailsignature.go backend/internal/modules/activities/signature.go backend/internal/modules/activities/signaturetemplate.go -->
# Your email signature

Open **Settings → Account → Edit signature**. Choose **Plain text** to write a
sign-off, or **HTML** to paste a formatted signature. Check the preview and choose
**Save**. The composer shows the same sign-off before you send.

Margince uses your own HTML first, then the company template, then your saved
plain text. Clear the HTML field, or choose **Plain text** and save, to use the
company template again. With none of these set, a send ends with a short greeting
and your name.

The greeting follows the message's language. When the message is too short to
tell, Margince uses the installation's language, then English.

The HTML preview shows what a send keeps. Images from websites and scripts are
removed. Only the built-in company logo can be embedded; other image attachments
are removed too.

If no text remains, the usual fallback signs the mail.
The settings row names the format you saved; check the preview for the result.
Fields such as `{name}` stay as typed in personal HTML. A plain message
also has a formatted version when the signature uses HTML.

The sign-off comes above any required notice and the unsubscribe footer. An email
an agent sends carries no sign-off. AI drafts never add one.
