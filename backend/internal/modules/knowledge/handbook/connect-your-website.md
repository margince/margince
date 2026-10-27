<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/webforms.tsx frontend/src/features/settings/webformdrawer.tsx frontend/src/features/settings/webformsetup.tsx frontend/src/features/settings/webformsnippet.ts backend/internal/modules/webforms backend/internal/compose/webformleads.go backend/internal/compose/webforms.go -->
# Connecting your website

A website form sends each question from your website to Margince as a new lead. Each form
has its own address. A page you write yourself, a site builder, a form tool or your
website's server can all send to it. Admin and Ops set
forms up in **Settings → Integrations**, under **Website forms**.

### How do I put a contact form on my website?
To put a contact form on your website, open **Settings → Integrations** and choose **New website form** in **Website forms**.
1. Type a **Name**, such as Contact page. Each lead's note shows it.
2. Keep **Sent by** on **Website**.
3. Under **Websites**, type each address the form is on, such as `https://www.example.com`, one per line. A page address such as `https://www.example.com/contact` counts as its whole website, and the field says so.
4. Choose **Create form**. The **Connect** dialog shows the **Form address** and the **Code to paste**.
5. Choose **Copy code** and paste the code into your page where the form goes.
Style the form as you like, but keep each field's `name`. A post from a website not on the list is refused.
Also called: contact form, request form, lead form, web form, embed a form.

### Can I keep the form I already have?
Yes. Point the form at the **Form address** with `method="post"`, and give its fields the names Margince reads:

| Field name | What it holds |
|---|---|
| `email` | The prospect's email address. Margince needs it. |
| `full_name`, or `first_name` and `last_name` | The prospect's name |
| `phone`, `company`, `title` | Phone number, company and job title |
| `message` | What the prospect wrote |
| `consent` | The consent tick box |
| `cf_` and a custom field's key, such as `cf_budget` | That lead custom field. **Show setup** lists the names. A value the field cannot take goes in the note. |

Margince also reads common names such as `name`, `your-email` or `Firma`. Every other field is kept in the lead's note, so nothing the prospect wrote is lost. Fields named `utm_source` and the other `utm_` names show where the prospect came from. Copy the hidden fields that start with `_mrg_` from the **Code to paste** as well: they stop most spam.
Also called: field names, map fields, custom fields, my own form, Contact Form 7.

### What happens when someone sends the form?
Margince adds a lead with the prospect's name, email, phone and company, and the source **Website**. Everything the prospect wrote goes into a note on the lead's timeline. If a lead or contact already has that email address, Margince adds the note to it and makes no new lead. A double click on the send button adds one note.
The prospect then sees the form's **Success message**, or goes to the **Thank-you page** if you set one.
Also called: where do my website leads go, new lead from website, duplicate lead.

### How do I connect WordPress?
To connect a WordPress site, add a **Custom HTML** block to the page and paste the **Code to paste** into it. Set **Sent by** to **Website** and list the site's address under **Websites**.

To keep the form an extension already shows, give its fields the names in the table above. Then set the form to post to the **Form address**. An extension that can only send its own kind of message cannot reach Margince this way. Use the **Custom HTML** block for that form instead.
Also called: WordPress extension, page builder, HTML block.

### How do I connect Webflow, Typeform or Tally?
To connect Webflow, Typeform or Tally, set **Sent by** to that tool when you create the form. Each tool signs what it sends with a secret, and Margince checks that secret.
1. In the tool, add a webhook for new form answers and paste the **Form address** as its address.
2. Copy the signing secret the tool shows for the webhook. In Typeform and Tally you type the secret yourself.
3. Paste the secret in **Signing secret** in Margince and choose **Create form**.
In Webflow, the webhooks are under the site's **Apps & integrations**. In Typeform they are under **Connect**, and in Tally under **Integrations**. A post whose secret does not match is refused.
Also called: Webflow form, Typeform webhook, Tally webhook, form builder.

### How do I send leads from my website's server?
To send leads from your own server, set **Sent by** to **Website server** and choose **Create form**. Margince shows the form's secret once. Store it on the server as `MARGINCE_FORM_SECRET`, then use the **Code to paste**. It signs each post, so a copy of an old post is refused.

This fits a site that handles its forms on the server, such as one built with Payload. **Rotate secret** in the row's **⋯** menu makes a new secret, and the old one stops working at once.
Also called: server to server, API, signed post, backend form.

### How do I connect a Payload site?
To connect a site built with Payload and its form tool, create a form with **Sent by** set to **Website server**. Then send each new form entry from a hook:

```ts
// payload.config.ts
import { formBuilderPlugin } from "@payloadcms/plugin-form-builder";
import { sendLead } from "./margince"; // the Code to paste, saved as margince.ts

formBuilderPlugin({
  formSubmissionOverrides: {
    hooks: {
      afterChange: [
        async ({ doc, operation }) => {
          if (operation !== "create") return;
          const fields = Object.fromEntries(
            doc.submissionData.map(({ field, value }) => [field, value]),
          );
          await sendLead(fields);
        },
      ],
    },
  },
});
```

Each Payload form field keeps the name you give it. Name them `email`, `full_name`, `company` and `message`, or Margince keeps them in the note.
Also called: Payload, form builder, Payload forms.

### How does Margince keep spam out?
Margince takes a post only from a website on the form's list, or with the form's secret. It also drops a post that fills the hidden `_mrg_hp` field, or one sent under three seconds after the page opened. Too many posts from one place in a minute are refused. A post the hidden checks drop still gets the usual answer, so a bot cannot learn which check stopped it.
Also called: spam, bots, junk leads.

### How do I ask for consent?
To ask for consent, edit the form and tick **Require consent before a submission is taken**. Type the text beside the tick box in **Consent wording**, and the version of your privacy policy if you have one. A post without the box ticked is refused. The note on each lead keeps the wording and the version, as a record of what the prospect agreed to. Consent to be contacted about a question is not consent to newsletters, so Margince does not add the lead to any mailing list.
Also called: GDPR, privacy tick box, DSGVO, opt-in.

### How do I stop a form?
To stop a form for a while, choose **Pause form** in the row's **⋯** menu. A paused form refuses posts until you choose **Resume form**. To stop it for good, choose **Archive**. An archived form's address stops working, and a new form gets a new address.
Also called: turn off form, disable form, delete form.
