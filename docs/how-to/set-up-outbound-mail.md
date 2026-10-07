# Set up outbound mail

Margince sends mail two ways, and an installation needs both set up before every
button that mails somebody works: the user's connected mailbox, and the SMTP
relay configured below.

## Which mail goes out which way

Mail a user writes goes out through **that user's connected mailbox**. The
composer, replies, scheduled sends and sequences all send as the rep, through
the Gmail, Microsoft 365 or IMAP connection they made under Settings →
Integrations. [connect-a-mailbox.md](connect-a-mailbox.md) sets that up.

Mail the installation writes by itself goes out through **the SMTP relay** in
the deployment file's `email:` block. It never uses anybody's connected mailbox,
even when the user who pressed the button has one. This covers:

| Mail | Started by |
|---|---|
| Privacy notice (GDPR Art. 14) | **Send privacy notice** on a "Privacy notice owed" card |
| Confirm-your-details link | **Ask them to confirm their details** on a contact |
| Double opt-in link for a purpose that needs confirming | the same button |
| Password reset | **Forgot password** on the sign-in page |
| Member invitation | Settings → Users & roles |
| Deal Room invitation | inviting a buyer to a Deal Room |
| Weekly review and morning brief by email | the worker's schedule, per the rep's Settings → Account choice |

The privacy notice and the confirmation links carry a single-use link that opens
the contact's own record. They go through the relay so that the link never sits
in a rep's Sent folder, where the rep or anyone else with access to that
mailbox could open it as the contact. The link is sealed in the keyvault and put
into the body only at the moment of sending.

## Configure the relay

Add an `email:` block to the deployment file (`config/margince.yaml` on a dev
stack, whatever `--config` / `MARGINCE_CONFIG` names elsewhere):

```yaml
email:
  enabled: true
  from_address: privacy@example.com
  smtp:
    host: smtp.example.com
    port: 587
    username: privacy@example.com
    password: ${env:MARGINCE_SMTP_PASSWORD}
```

- `from_address` is the sender every recipient sees. Use an address of the
  company that runs this installation. That company is the data controller, and
  a privacy notice must come from the controller.
- `password` takes the reference form, `${env:NAME}` or `${file:/path}`, never
  the value itself. Leave it out for a relay that needs no login. The first boot
  that reads it seals it into the keyvault, and later boots read it from there;
  [configuration.md](../reference/configuration.md) explains how to take the
  reference out afterwards.
- A Gmail or Google Workspace account can serve as the relay:
  `smtp.gmail.com`, port `587`, the account address as `username`, and a Google
  **app password** (not the account password) as `password`.

Boot refuses a block with `enabled: true` and no `smtp.host`, a port outside
1–65535, or an invalid `from_address`.

Two more settings have to be in place:

- **`--public-base-url` / `MARGINCE_PUBLIC_BASE_URL`** must be set. The api
  refuses to boot with `email.enabled` and no base URL, because the links in
  these messages are built on it. With a real sender configured it must be an
  https address a recipient can open; `MARGINCE_ENV=dev` admits the dev stack's
  `http://localhost`, which only you can open.
- Both the api and the worker must read **the same deployment file**, because they
  split the work. The api sends password resets and invitations itself, at the
  moment of the request. For the privacy notice and the confirm links, the api
  only stages the message, and only when it has a relay configured; `cmd/worker`
  then transmits it. The worker also sends the weekly review and morning brief.

The api does not reload its configuration. Restart both processes (`make dev`
on a dev stack). The api's boot log then prints
`api operator mail enabled (password reset, invites)`, and the worker's prints
`weekly review mail on (...)` instead of `weekly review mail off (no operator
mail configured)`.

## What happens without a relay

Nothing refuses to boot. Every mail in the table above fails without notice or
with a one-line message:

- **Send privacy notice** and **Ask them to confirm their details** answer
  "Not sent: this installation cannot send mail to *address*." The link is
  created and not sent. The privacy-notice duty stays open, and its one-month
  deadline keeps running.
- **Forgot password** is absent from the sign-in page.
- An invited member has no way to set a password; Settings → Users & roles
  offers **Get set-password link** to hand over by other means.
- The weekly review and morning brief stay on Home and are not mailed.

A dev stack from `make dev` has no relay: `config/margince.example.yaml` ships
the block commented out. To discharge a privacy-notice duty on such a stack,
either configure a relay as above, or tell the contact by other means and close
the duty with **End the duty…**, recording how you told them.
