# Connect a shared mailbox

A shared mailbox, such as `sales@` or an operations desk, belongs to no one. An admin connects it, and one team
reads the mail it captures. Each provider needs its own setup on the provider's side before the connect. This page
covers that setup for IMAP, Microsoft 365 and Google Workspace, and the request that connects the mailbox.

For a seat's own mailbox, see [connect-a-mailbox.md](connect-a-mailbox.md).

## What every provider needs

- `MARGINCE_KEYVAULT_ROOT_KEY` on the API and the worker. The credential is sealed in the vault, and it is deleted
  when the mailbox is disconnected.
- A signed-in admin. The routes refuse an agent passport.
- A reader team with at least one active member.

The connect is `POST /v1/shared-mailboxes`. The body names the `provider`, the `address`, the `reader_team_id` and
one credentials member named after the provider. The contract is in `backend/api/crm.yaml` under
`ConnectSharedMailboxRequest`. A second connect of the same provider and address reconnects the stored mailbox and
keeps its mail and readers.

The body may also name a `backup_owner_id`, the next section says what it does. Margince checks the credentials
before it stores anything. A refused check answers `422` with a code that names
what to fix:

| Code | What to check |
|---|---|
| `shared_mailbox_credentials_invalid` | A field is missing or malformed: the tenant id, the key file, the address |
| `shared_mailbox_grant_refused` | The provider refused to issue a token: secret, consent or delegation |
| `shared_mailbox_mailbox_denied` | A token was issued, but it does not reach this mailbox or its user record |
| `shared_mailbox_mailbox_not_found` | The provider knows no mailbox at this address |
| `shared_mailbox_foreign_domain` | Google Workspace only: the address is outside the installation's own domains |

A provider that does not answer gives `502 shared_mailbox_unreachable`. No answer repeats the provider's own message
or the secret.

## Who gets the work from shared mail

Mail that only a shared mailbox captured can still make work: a reply reminder, a row on the waiting list, a
promise, or the question which deal a mail is about. `SharedMailWorkOwnerSQL` in
`backend/internal/platform/database/storekit/sharedmailowner.go` picks the one colleague who gets it, in this order:

1. The owner of the sender's contact. For mail the mailbox sent, the owner of the contact it went to.
2. The owner of that contact's current company.
3. The mailbox's backup owner.

A shared mailbox's own address never counts as the sender or a recipient. When a step names two different
colleagues, for example mail the mailbox sent to two contacts with different owners, that step gives no answer and
the backup owner is asked. When the mail sits in two shared mailboxes with different backup owners, nobody gets the
work.

When the answer changes, for example the contact gets a new owner, the next run of the reply reminder pass moves an
untouched reminder to the new colleague. It retires the reminder when nobody answers the mail any more. A reminder a
colleague edited, completed or archived stays as it is.

A colleague is passed over when they cannot hold work, for example an agent or a read-only seat. A colleague is also
passed over when they cannot already read the mail. Held mail is read by the reader team and the mail's own
participants, so a contact owner outside the reader team is skipped. When nobody is left, the mail makes no work.

The backup owner must be an active member of the reader team who can hold work. Anyone else answers
`422 backup_owner_not_in_reader_team`. Set or change it with `PATCH /v1/shared-mailboxes/{id}` and
`backup_owner_id`, and send `null` to remove it. That change alone needs no `confirm: true`, because it moves no
mail. A backup owner who leaves the reader team, or leaves the company, gets no more work: the order above checks
membership each time it runs. Moving the mailbox to another reader team removes a backup owner that team does not
hold.

A reply reminder needs mail open to the workspace. Held mail is never sent to a model to judge whether it asks
for an answer, so held mail gets no reply reminder from anyone.

A sender with no contact is filed under nothing and gets no contact, with or without a backup owner.

## IMAP

IMAP needs no setup on the provider's side beyond a login for the mailbox. Use an app password where the server
offers one.

```sh
curl -sS -X POST "$API/v1/shared-mailboxes" -b "$COOKIE" -H 'Content-Type: application/json' -d '{
  "provider": "imap", "address": "sales@example.com", "reader_team_id": "<team-id>",
  "imap": {"host": "imap.example.com", "username": "sales@example.com", "secret": "<app-password>"}
}'
```

Margince logs in once to check the password. A refused login answers `422 imap_login_rejected`.

## Microsoft 365

Margince reads the mailbox with an app registration in the customer's own Entra tenant. The app holds an
application permission, so no user signs in and no refresh token is stored.

1. In the Entra admin center, open **App registrations** and register a new app. Single tenant is enough.
2. Note the **Directory (tenant) ID** and the **Application (client) ID**.
3. Under **Certificates & secrets**, add a client secret. Note its value now: Entra shows it once.
4. Under **API permissions**, add **Microsoft Graph**, **Application permissions**, `Mail.Read` and
   `User.ReadBasic.All`.
5. Select **Grant admin consent** for the tenant.
6. Limit the app to the shared mailbox in Exchange Online, as the next section shows.

### Limit the app to the mailbox

`Mail.Read` as an application permission reads every mailbox in the tenant. Exchange Online can narrow it to the
mailboxes you choose. Use RBAC for Applications in Exchange Online PowerShell:

```powershell
New-ServicePrincipal -AppId <client-id> -ObjectId <enterprise-app-object-id> -DisplayName "Margince shared mail"
New-ManagementScope -Name "Margince shared mailboxes" -RecipientRestrictionFilter "PrimarySmtpAddress -eq 'sales@example.com'"
New-ManagementRoleAssignment -App <client-id> -Role "Application Mail.Read" -CustomResourceScope "Margince shared mailboxes"
```

The object id is the one on the app's **Enterprise applications** entry, not on its registration. With the Exchange
role in place, remove the `Mail.Read` grant from **API permissions**. Otherwise the tenant-wide grant still applies.
Keep `User.ReadBasic.All`: it reads only names and ids, never mail.
`Test-ServicePrincipalAuthorization -Identity <client-id> -Resource sales@example.com` shows whether the app reaches
a mailbox. Exchange can take up to two hours to apply a change.

### Connect it

```sh
curl -sS -X POST "$API/v1/shared-mailboxes" -b "$COOKIE" -H 'Content-Type: application/json' -d '{
  "provider": "graph", "address": "sales@example.com", "reader_team_id": "<team-id>",
  "graph": {"tenant_id": "<tenant-id>", "client_id": "<client-id>", "client_secret": "<secret>"}
}'
```

The tenant id is the directory id or a verified domain of the tenant. Margince refuses the multi-tenant
aliases, because an app token comes from one tenant only.

The check mints an app token, reads the mailbox's user object id, and opens the mailbox's inbox. Margince needs the
id because Graph may name the mailbox by it in its links. Without `User.ReadBasic.All` the check answers
`422 shared_mailbox_mailbox_denied`.

### Notifications and polling

Margince asks Graph for change notifications on the mailbox when the installation has a notification URL. If Graph
refuses that subscription, the mailbox still syncs on the regular poll through its delta link. A shared mailbox asks
Graph for pages of at most 25 messages and saves its place after each page. So a slow or throttled mailbox keeps the
mail it already read.

### Rotate or revoke the secret

To rotate, add a second client secret in Entra, then connect the mailbox again with the new value. The reconnect
keeps the mailbox, its readers and its mail. Delete the old secret in Entra after the reconnect succeeds.

To revoke, delete the secret or the app in Entra. The next sync is refused, and the mailbox shows as needing a
reconnect. To stop capture from Margince, use `POST /v1/shared-mailboxes/{id}/disconnect`, which also deletes the
sealed secret.

## Google Workspace

Margince reads the mailbox through a service account that the Workspace admin lets act for the mailbox. This is
domain-wide delegation, with the read-only Gmail scope alone.

1. In the Google Cloud console, choose or create a project and turn on the **Gmail API**.
2. Under **IAM & Admin**, **Service accounts**, create a service account. It needs no project role.
3. Open the service account, then **Keys**, **Add key**, **Create new key**, **JSON**. Keep the downloaded file safe.
4. Note the service account's **Unique ID**: this is the client id that the next step needs.
5. In the Google Admin console, open **Security**, **Access and data control**, **API controls**, then **Manage
   Domain Wide Delegation**.
6. Add a new entry with that client id and the scope `https://www.googleapis.com/auth/gmail.readonly`.

The service account can then read every mailbox in the Workspace domain. Margince reads only the address you
connect, and it accepts only an address on one of the installation's own domains. Add the domain under own domains
first if the connect answers `shared_mailbox_foreign_domain`.

### Connect it

The `service_account_key` field holds the whole JSON key file as one string.

```sh
curl -sS -X POST "$API/v1/shared-mailboxes" -b "$COOKIE" -H 'Content-Type: application/json' -d "$(jq -n \
  --arg team "<team-id>" --rawfile key service-account.json \
  '{provider: "gmail", address: "sales@example.com", reader_team_id: $team, gmail: {service_account_key: $key}}')"
```

The check mints a token for the mailbox and reads its Gmail profile. A delegation that is missing or names another
scope answers `422 shared_mailbox_grant_refused`.

### Rotate or revoke the key

To rotate, create a second JSON key on the service account, and connect the mailbox again with the new file. Delete
the old key under **Keys** once the reconnect succeeds. Google keeps both keys valid until you delete one.

To revoke, delete the key, or remove the client id from **Manage Domain Wide Delegation**. The second way stops the
service account for every mailbox at once. The next sync is refused, and the mailbox shows as needing a reconnect.
To stop capture from Margince, disconnect the mailbox, which also deletes the sealed key.
