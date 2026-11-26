# Connect Google groups to the group role map

The group role map gives a signed-in member the roles of their directory groups. Microsoft sends a member's groups
inside the sign-in token. Google does not, so for a member who signs in with Google, Margince asks the Google
Workspace Directory API for the groups. That needs a service account that the Workspace admin lets act for one
admin, and an admin who stores its key in Margince.

Without the connection the map still works for Microsoft, and a Google member gets no roles from it.

## What it does and does not do

- The lookup runs at a Google sign-in, only for a member who was already invited, and only while the map has at least
  one entry and a key is stored.
- A token that carries groups wins, and no lookup runs.
- Only a Workspace account whose token names the admin's own domain (`hd`) is looked up. A personal Google account,
  or one from another domain, gets no groups. The lookup asks for the account's Google subject, which for a Workspace
  user is the Directory user id, so a reused address never reads the groups of whoever held it before.
- A failed or slow lookup (five seconds at most) signs the member in with no new roles. The map only adds roles, so a
  failure takes nothing away. Nothing is retried inside the sign-in.
- Each failure writes one `system_log` row with the action `google_directory` and a reason class. Settings →
  **Sign-in and security** → **Group role grants** shows the newest result.
- Only groups the member belongs to directly count. The Directory API's `userKey` query does not resolve groups nested
  inside other groups.
- A group is named by its email address. Enter it in the map as Google lists it, for example
  `sales@example.com`.

## Set it up in Google

1. In the Google Cloud console, choose or create a project and turn on the **Admin SDK API**.
2. Under **IAM & Admin**, **Service accounts**, create a service account. It needs no project role.
3. Open the service account, then **Keys**, **Add key**, **Create new key**, **JSON**. Keep the downloaded file safe.
4. Note the service account's **Unique ID**: this is the client id that the next step needs.
5. In the Google Admin console, open **Security**, **Access and data control**, **API controls**, then **Manage
   Domain Wide Delegation**.
6. Add a new entry with that client id and the one scope
   `https://www.googleapis.com/auth/admin.directory.group.readonly`.
7. Choose the Workspace admin the service account acts for. The Directory API refuses a delegated read that names no
   admin, and the admin must be able to read groups.

Domain-wide delegation is a strong grant on Google's side. The scope above is read-only and Margince requests no
other.

## Store the key in Margince

Open Settings → **Sign-in and security** → **Group role grants**. In **Google group lookup**, paste the key file or
choose it, enter the admin's address, and choose **Check and store**. Only an admin can do this.

Margince asks Google for the admin's own groups before it stores anything. A key that fails is refused with the
reason, and nothing is sealed. A stored key is sealed in the key vault, so the installation needs
`MARGINCE_KEYVAULT_ROOT_KEY`. Nothing returns the key afterwards.

The same call is `PUT /v1/installation/authentication-policy/google-directory` with `service_account_json` and
`admin_subject`. `GET` on that path says whether a key is stored and how the newest check of that key went, and `DELETE` removes
it.

| Reason | What to check |
|---|---|
| `key_invalid` | The stored key file cannot sign. Store the key again |
| `delegation_refused` | The client id has no delegation for the scope above, or the admin address is wrong |
| `permission_denied` | The admin cannot read groups |
| `member_not_found` | Google does not know the signed-in member's address |
| `timeout`, `unavailable` | Google did not answer in time, or limits requests |
| `bad_answer` | Google answered in a form Margince cannot read |

## Rotate or remove the key

To rotate, create a second JSON key on the service account and store it with **Check and store**. Margince seals the
new key before it destroys the old one. Delete the old key under **Keys** afterwards.

To remove the connection, choose **Remove connection**. Google members then sign in as before, with no roles from the
map. Removing the delegation entry in the Google Admin console stops the service account at once, and the next
lookup fails with `delegation_refused`.
