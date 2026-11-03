<!-- prose:plain -->
# Mail connectors and sign-in

The Google and Microsoft apps behind mail capture and sign-in, and how to turn the password method
off. The settings themselves are in [Every setting](configuration.md#every-setting).

## Capture connector OAuth (api, worker): Gmail / Microsoft 365

To turn on the Gmail and Outlook/M365 capture connectors, the operator gives their own OAuth app.
Without these, `make dev` does not change, and the `/connectors/gmail/*` / `/connectors/graph/*`
surfaces stay their declared 501. Secrets go through the environment, never CLI flags in production
(any user can read argv). Roles: **api** serves connect/callback, **worker** runs the background
sync.

- **The apps.** `MARGINCE_GMAIL_CLIENT_ID` and `MARGINCE_GMAIL_CLIENT_SECRET` are the Google app;
  `MARGINCE_GRAPH_CLIENT_ID` and `MARGINCE_GRAPH_CLIENT_SECRET` the Microsoft (Entra) app. Both
  roles read them. An admin can store each app under Settings instead, with no restart. A pair the
  deployment sets wins over the stored app and locks it.
- **One Microsoft app** serves Outlook mail (`/connectors/graph/*`) and Outlook calendar
  (`/connectors/graphcal/*`). Grant it the mail permissions (`Mail.Read`, `Mail.Send`) and
  `Calendars.ReadWrite`, and register a redirect URI for each. They are separate connections with
  separate consents.
- **`--graph-tenant`** is the Microsoft identity tenant of capture, `common` (any company) when
  unset. It takes one authority, never a list.
- **`--connector-state-key`**, on the api, is the HMAC key that signs the OAuth connect `state`. It
  must be at least 32 bytes, and both connect steps need it.
- **`--api-base-url`** is the base of the api the outside can reach, for the OAuth callback
  `redirect_uri`; it defaults to `--public-base-url`. Set it only when the api and the app each have
  their own origin, as in dev. Google sign-in (`/auth/oidc/google/*`) uses this `redirect_uri` too.
  Add it to the redirect URIs of the Google app in its cloud settings, or every try ends in
  `redirect_uri_mismatch`.
- **`--mcp-apps-base-url`** is the origin the api reads the MCP App view documents from, at start
  and again from time to time. It defaults to `--public-base-url`, and only matters while
  `mcp.connector_enabled` is on. The api itself must reach it, which may differ from what the
  public reaches; a CDN origin works. The scheme must be `https` unless the host is a loopback or
  private address. A value the fetch would refuse stops the boot and names the setting.
- **Gmail push.** `--gmail-pubsub-topic` on the worker (`projects/<p>/topics/<t>`) turns on the
  job that registers and renews the push watch; unset, capture polls. `--gmail-push-token` on the
  api is the shared secret on the push URL and turns on `POST /webhooks/gmail`. Set
  `--gmail-push-audience` and `--gmail-push-service-account` together, and the webhook also checks
  the Google OIDC token. `--gmail-jwks-url` is for test and dev only.
- **Graph push.** `--graph-notification-url` on the worker is the public URL Microsoft posts change
  notices to, token included (`https://<api>/webhooks/graph?token=…`). `--graph-push-token` on the
  api turns on `POST /webhooks/graph`, and must be the same token. It is the only check at that
  route, because Microsoft signs nothing on a change notice.


### Microsoft sign-in tenants

`--microsoft-signin-tenant`, on the api, lists the Entra directory IDs, joined by `,`. Their members
may sign in through `/auth/oidc/microsoft/*`, on the same app as Graph capture. Unset, it takes
`--graph-tenant` when that names a directory. With neither, a Microsoft app stored under Settings
signs members in on the directory it is pinned to. An app with no pin signs nobody in.
Add the callback the api prints at boot (`<api-base>/v1/auth/oidc/microsoft/callback`) to the
redirect URIs of the Entra app, and grant it `openid profile email`.

- **Sign-in cannot run on `common`/`organizations`/`consumers`.** Sign-in matches the address on
  the token to a member that is already there. The admin of any Entra tenant can set the `mail`
  field of any of their users to any string. An open authority would let any user who can create a
  tenant sign in here as any member. Each ID is a directory whose admins this installation trusts.
  An alias leaves the provider off, with the reason in the boot log.
- **Routing.** One work directory routes the browser through the own authority of that directory.
  Personal accounts alone go through `consumers`, many work directories through `organizations`,
  and a list with both kinds through `common`. The routing never decides what is accepted; the
  `tid` check against the list does.
- **The registration audience must reach that authority**, or Microsoft refuses at its
  sign-in step and the callback never runs. One directory works under any audience. Many work
  directories need at least `Accounts in any organizational directory` (`AzureADMultipleOrgs`). A
  list that names personal accounts needs `…and personal Microsoft accounts`
  (`AzureADandPersonalMicrosoftAccount`). The audience belongs to the app registration. So to add to
  the list without a change to the registration fails at Microsoft.
- **Personal Microsoft accounts** sign in when the list names their tenant,
  `9188040d-6c67-4c5b-b112-36a304b66dad`. No admin stands over a consumer tenant. So the address is
  one where Microsoft made the holder prove they get mail: the same test this installation accepts
  for a password reset. Their `preferred_username` is not accepted as an address, because it is a
  handle the holder picks. A work account UPN is not like that: it sits on a domain a tenant proved
  by DNS.

### Turning the password method off

An installation that signs its members in through an identity provider turns off password sign-in
in `margince.yaml`:

```yaml
auth:
  password:
    enabled: false   # default true
```

With it off, `POST /v1/auth/login` and `POST /v1/auth/forgot-password` answer **501** naming the
method. `/v1/auth/capabilities` reports `password: false` and `password_reset: false`, and the
login screen shows only the provider buttons. The set-password link an admin sends still works. It
gives a seat, not a way in. An installation that turns the method back on must not have to give
every seat again first.

**The api refuses to boot** with the method off and no federated provider mounted, since that
deployment has no way in at all. The check needs a mounted provider, which is less than "someone can
sign in today". A provider whose OAuth app an admin has not stored yet is mounted and shows no
button. The login screen says so, and does not show an empty card.
