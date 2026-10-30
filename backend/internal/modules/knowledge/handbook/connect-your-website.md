<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/webforms.tsx frontend/src/features/settings/webformdrawer.tsx frontend/src/features/settings/webformsetup.tsx frontend/src/features/settings/webformsnippet.ts backend/internal/modules/webforms backend/internal/compose/webformleads.go backend/internal/compose/webforms.go -->
# Connecting your website

A website form sends each question from your website to Margince as a new lead. Each form
has its own address. A page you write yourself, a site builder, a form tool or your
website's server can all send to it. Admin and Ops set
forms up in **Settings → Website forms**.

### How do I put a contact form on my website?
<!-- opens: settings/web-forms -->
To put a contact form on your website, open **Settings → Website forms** and choose **New website form**.
1. Under **Where is your form?**, keep **Your website**.
2. Type a **Name**, such as Contact page. Each lead's note shows it.
3. Under **Websites**, type each address the form is on, such as `https://www.example.com`, one per line. A page address such as `https://www.example.com/contact` counts as its whole website, and the field says so.
4. Choose **Create form**. The **Connect** dialog lists the steps, each with what to copy.
5. Choose **Copy code** and paste the code into your page where the form goes. In a site builder, set the form to send to the **Form address** instead.
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
Margince adds a lead with the prospect's name, email, phone and company, and the source **Website**. Everything the prospect wrote goes into a note on the lead's timeline, shown as a quote. If a lead or contact already has that email address, Margince adds the note to it and makes no new lead. A double click on the send button adds one note. If the address was erased on request, Margince keeps nothing, and the prospect sees the same message as any other.

A line outside the quote was written by Margince, so a prospect cannot type one.

The prospect then sees the form's **Success message**, or goes to the **Thank-you page** if you set one.
Also called: where do my website leads go, new lead from website, duplicate lead.

### Who gets the leads from a form?
Pick a colleague in **Owner of new leads** when you create or edit the form. Each new lead from it then goes to that colleague. With **Unassigned**, anyone on the team can pick the lead up. If the owner leaves the workspace, new leads arrive unassigned until you pick someone else.
Also called: assign website leads, lead owner, route form leads.

### How do I connect WordPress?
To connect a WordPress site, add a **Custom HTML** block to the page and paste the **Code to paste** into it. Choose **Your website** and list the site's address under **Websites**.

To keep the form an extension already shows, give its fields the names in the table above. Then set the form to post to the **Form address**. An extension that can only send its own kind of message cannot reach Margince this way. Use the **Custom HTML** block for that form instead.
Also called: WordPress extension, page builder, HTML block.

### How do I connect a Webflow form?
To connect a Webflow form, choose **Your website** and list the Webflow site's address under **Websites**. Then, in Webflow, open the form's settings. Set **Action** to the **Form address** and **Method** to **POST**, and publish the site. Name the email field `email`.

A webhook made in the settings of Webflow is not signed, so Margince cannot check it. Choose **Webflow webhook** only when someone who writes code makes the webhook with the Webflow API. Create the form first and give its **Form address** to the webhook.

The Webflow API then answers the secret of the webhook once. A webhook an OAuth app makes is signed with the client secret of that app instead. Paste the secret in **Signing secret** under **Edit**. Until then the form takes no posts, and its row says **Waiting for its secret**.
Also called: Webflow form, Webflow action, Webflow webhook.

### How do I connect Typeform or Tally?
To connect Typeform or Tally, choose that tool under **Where is your form?** and choose **Create form**. Margince makes the form's secret and shows it once in the **Connect** dialog.
1. In Typeform, open the form and go to **Connect**, then **Webhooks**. In Tally, open the form and go to **Integrations**, then **Webhooks**.
2. Paste the **Form address** as the address of the webhook.
3. Paste the form secret: in Typeform under **Secret**, in Tally under **Signing secret**. Save the webhook. Typeform makes a new webhook switched off, so switch it on.
4. Send a test answer. It shows up as a new lead under **Leads**. An email Margince already has gets a note on that record instead.

A post whose secret does not match is refused. If the secret is lost, choose **Make a new secret** in the row's **⋯** menu and paste the new one in the tool.
Also called: Typeform webhook, Tally webhook, form builder.

### How do I send leads from my website's server?
To send leads from your own server, choose **Website server** and choose **Create form**. Margince shows the form's secret once. Store it on the server as `MARGINCE_FORM_SECRET`, then use the **Code to paste**. It signs each post, so a copy of an old post is refused.

This fits a site that handles its forms on the server, such as one built with Payload. **Make a new secret** in the row's **⋯** menu makes a new secret, and the old one stops working at once.
Also called: server to server, API, signed post, backend form.

### How do I connect a Payload site?
To connect a site built with Payload and its form tool, create a form with **Website server** chosen. Then send each new form entry from a hook:

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
To ask for consent, edit the form and tick **Require consent before a submission is taken**. Type the text beside the tick box in **Consent wording**, and the version of your privacy policy if you have one. A post without the box ticked is refused. The note on each lead keeps the wording and the version, above the quote, as a record of what the prospect agreed to.

If the box is not required and the prospect leaves it empty, the note says **Consent: not ticked**. Consent to be contacted about a question is not consent to newsletters, so Margince does not add the lead to any mailing list.
Also called: GDPR, privacy tick box, DSGVO, opt-in.

### How do I stop a form?
To stop a form for a while, choose **Pause form** in the row's **⋯** menu. A paused form refuses posts until you choose **Resume form**. To stop it for good, choose **Archive**. An archived form's address stops working, and a new form gets a new address.
Also called: turn off form, disable form, delete form.
