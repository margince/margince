<!-- prose:plain -->
# Set up outbound mail

Margince sends mail in two ways, and an installation needs both set up before every
button that sends mail works. One is the connected mailbox of the user, and the other is the SMTP
relay that you set up below.

## Which mail goes out which way

Mail a user writes goes out through **the connected mailbox of that user**. The
composer, replies, planned sends and sequences all send as the rep, through
the Gmail, Microsoft 365 or IMAP connection they made under Settings →
Integrations. To set that up, see [connect-a-mailbox.md](connect-a-mailbox.md).

Mail the installation writes by itself goes out through **the SMTP relay** in
the `email:` block of the deployment file. It never uses the connected mailbox of a user,
even when the user who clicked the button has one. This covers:

| Mail | Started by |
|---|---|
| Privacy notice (GDPR Art. 14) | **Send privacy notice** on a "Privacy notice owed" card |
| Confirm-your-details link | **Ask them to confirm their details** on a contact |
| Double opt-in link for a purpose that needs confirming | the same button |
| Password reset | **Forgot password** on the sign-in page |
| Member invitation | Settings → Users & roles |
| Deal Room invitation | inviting a buyer to a Deal Room |
| Weekly review and morning brief by email | the schedule of the worker, as the rep sets it in Settings → Account |

The privacy notice and the confirm links carry a link that works one time, and that opens
the contact's own page for that message. They go through the relay so that the link never sits
in the Sent folder of a rep. There, the rep, or any other user with access to that
mailbox, could open it as the contact. The keyvault seals the link, and the link goes
into the body only at the time of sending.

## Set up the relay

Add an `email:` block to the deployment file. That is `config/margince.yaml` on a dev
stack, and the file that `--config` / `MARGINCE_CONFIG` names in other places:

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
- `password` takes the reference form, `${env:NAME}` or `${file:/path}`, and never
  the value itself. Leave it out for a relay that needs no login. The first start
  that reads it seals it into the keyvault, and later starts read it from there.
  To take the reference out after that, see [configuration.md](../reference/configuration.md).
- Once a password is sealed, leaving it out keeps the sealed copy in use. To stop
  using it (a new relay with no login, or a password that is no longer secret),
  write `password: ${none}`. The next start deletes the sealed copy and sends
  without a login.
- A Gmail or Google Workspace account can be the relay. Use
  `smtp.gmail.com` and port `587`. Set the account address as `username`, and a Google
  **app password** (not the account password) as `password`.

The start refuses a block with `enabled: true` and no `smtp.host`, a port outside
1–65535, or a `from_address` that is not valid.

Two more settings must be in place:

- **`--public-base-url` / `MARGINCE_PUBLIC_BASE_URL`** must be set. The API
  refuses to start with `email.enabled` and no base URL, because the links in
  these messages are built on it. With a real sender set up, it must be an
  HTTPS address that a recipient can open. `MARGINCE_ENV=dev` allows the
  `http://localhost` of the dev stack, which only you can open.
- Both the API and the worker must read **the same deployment file**, because they
  share the work. The API sends password resets and invitations itself, at the
  time of the request. For the privacy notice and the confirm links, the API
  only puts the message in line, and only when it has a relay set up. Then `cmd/worker`
  sends it. The worker also sends the weekly review and the morning brief.

The API does not read its settings again while it runs. Start both programs again (`make dev`
on a dev stack). Then the start log of the API prints
`api operator mail enabled (password reset, invites)`. The log of the worker prints
`weekly review mail on (...)`, and not `weekly review mail off (no operator mail configured)`.

## What happens without a relay

Nothing refuses to start. Every mail in the table above fails, with no sign or
with a message of one line:

- **Send privacy notice** and **Ask them to confirm their details** answer
  "Not sent: this installation cannot send mail to *address*." The system makes
  the link and does not send it. The privacy notice duty stays open, and its deadline of one month
  keeps running.
- **Forgot password** is not on the sign-in page.
- An invited member has no way to set a password. Settings → Users & roles
  offers **Get set-password link**, so you can hand the link over in another way.
- The weekly review and the morning brief stay on Home, and no one mails them.

A dev stack from `make dev` has no relay: `config/margince.example.yaml` ships
the block as a comment. To close a privacy notice duty on such a stack,
set up a relay as above. Or tell the contact in another way, and close
the duty with **End the duty…**, with a note of how you let them know.
