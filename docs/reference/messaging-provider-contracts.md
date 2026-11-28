<!-- describes: backend/internal/modules/chatlinks backend/internal/compose/chatsync backend/internal/compose/messengeroauth.go backend/internal/compose/messengeraccounts backend/internal/compose/teamschannelsetup.go -->
# Messaging provider contracts

Margince has two provider capabilities: attended Messenger and shared channel sync. Their public
operations are declared in [the API contract](../../backend/api/crm.yaml). The transport adapters
share [the conversation boundary](../../backend/internal/modules/chatlinks/messaging) and
[the canonical channel boundary](../../backend/internal/modules/chatlinks/chatwire).

## Capability boundaries

| Provider | Messenger | Channel reads | Automatic channel posts |
|---|---|---|---|
| Dispact | Personal participant authorization, private conversations and replies | Immutable channel and complete canonical thread capabilities | Effective complete audience required |
| Slack | User authorization, direct and group conversations, joined channel discovery | Complete bounded threads under the standard history tier | Bot installation and a verified internal audience |
| Teams | Delegated personal authorization, chats and replies | Standard, private and shared channels with verified app access | Bot Framework delivery after channel and audience checks |

Private Teams channel access does not follow from installation in the parent team. Channel-specific installation and consent must be qualified before promising private reads.

The Teams adapter pins the selected tenant and the administrator-approved installed teams. Shared channel rosters use effective membership and reject unknown tenant identities for outgoing sharing.
It supports the public-cloud Graph and Bot Framework endpoints named in
[the adapter](../../backend/internal/modules/chatlinks/teams). Other clouds are not accepted by
substituting a URL.

Scopes, group limits and native manifest permissions live in the provider adapters:
[Dispact](../../backend/internal/modules/chatlinks/dispact),
[Slack](../../backend/internal/modules/chatlinks/slack) and
[Teams](../../backend/internal/modules/chatlinks/teams). Their declarations, rather than a copied
scope list here, determine authorization requests and scope validation. Personal account OAuth and
refresh composition live in [messengeraccounts](../../backend/internal/compose/messengeraccounts).

## Workspace service selection

The selected service is durable configuration, independent of whether a shared or personal account is connected. Both credential writers acquire the same workspace lock and refuse another provider. An administrator can change the selection; it blocks old consent callbacks before disconnecting old credentials and ending links. Cleanup holds the exact selection version, so a concurrent switch or a switch away and back cannot withdraw a newer selection. Disconnect first records a disabled selection to reject late callbacks. A failed cleanup can be retried. Ended links retain History and release their records for relinking. WhatsApp and other customer messaging channels are outside this team-service choice.

## Dispact canonical reads

Existing bot credentials remain separate from personal participant credentials. Canonical channel
sync requires the capabilities checked by
[CanonicalAvailable](../../backend/internal/modules/chatlinks/dispact/canonical.go). Outgoing sharing
also needs the effective audience reader.

An immutable channel path uses `id:<uuid>`. Response IDs remain UUIDs; they are not replaced with
that path namespace. A UUID-shaped legacy slug keeps its legacy meaning. Existing legacy message
timestamps remain write handles where the bot write API requires them.

The selected-message source lookup returns channel, message and root identities without a body.
Margince then reads the complete bounded canonical thread, checks the selected message and rechecks
the source after summarization. The lookup and exact response validation are in
[the source locator](../../backend/internal/modules/chatlinks/dispact/location.go).
An unavailable capability leaves the operation unavailable rather than guessing a root or audience.

## Slack history profile

Canonical channel sync supports Marketplace-approved apps and internal apps with Slack's standard
history tier. Commercially distributed non-Marketplace apps with the one-request-per-minute history
limit cannot complete the adapter's immediate full-thread validation and are not a supported
canonical sync profile. This boundary is also shown during shared Slack installation.

Slack documents the distribution distinction and reduced history/replies limits in its
[rate-limit change](https://docs.slack.dev/changelog/2025/05/29/rate-limit-changes-for-non-marketplace-apps/)
and [clarification](https://docs.slack.dev/changelog/2025/06/03/rate-limits-clarity/).
The current [replies method](https://docs.slack.dev/reference/methods/conversations.replies/)
declares bot and user history scopes. Do not infer a channel-read capability from a successful
message post alone.

Bot thread reads use the bot credential. Personal and bot history reads share the app/workspace quota. An administrator can record the distribution independently of a shared bot installation. Unknown personal profiles use the reduced tier. Personal history reads do not fetch a full member roster on every page; the provider enforces read access. Sends and outcome drafts separately verify current recipients and access. Immediate outcome drafts are unavailable for reduced profiles because source verification requires repeated reads. Public channels can be linked read-only; automatic record sharing requires a private internal channel.

A provider 429 carries its retry interval through the shared HTTP boundary. Work remains pending;
there is no partial recap or fabricated coverage success. A bounded thread that cannot be read in
full is refused. Live qualification must use the actual registered app profile.

## Teams notifications and delivery

Teams channel notifications carry metadata only. Subscription failures leave polling active; channel-specific consent does not guarantee notification subscription access. The receiver binds app, tenant, subscription,
channel and sealed client state before queuing a canonical reread. Validation challenges use the
provider's required plain response. The subscription also supplies the lifecycle callback required
for its lifetime by [Microsoft's lifecycle contract](https://learn.microsoft.com/en-us/graph/change-notifications-lifecycle-events).

A lease bound is stored before creating or renewing a subscription. A lost response therefore cannot
make cleanup assume the remote subscription never existed. The cleanup dispatcher includes
connections that normal bot polling no longer visits. Retirement records whether removal was
confirmed or only the bounded lease expired, then destroys the detached credential.

Automatic Teams posts use the installed bot's validated conversation reference and service URL.
The channel receipt is retained for an update of that message. Delegated Graph chat sends are the
separate attended Messenger path, not a substitute for unattended posting.

## Local records and qualification

Private Messenger bodies are fetched on demand. Durable attempts keep identifiers, hashes and
minimal receipt state. Checking a receipt rereads that durable attempt; it does not perform a
provider search for a lost acknowledgement. An uncertain attempt is never automatically resent.

Channel recaps and delivery records follow the existing record, activity, restriction and retention
boundaries. Unlink stops association work but does not prove a provider copy was erased. Successful
provider rewrites and unverified withdrawals retain different evidence.

Deterministic provider HTTP fixtures exercise these paths without accounts. Live qualification
still needs isolated app registrations for sender identity, private replies, complete audiences,
consent renewal, revocation, rate limits and supported provider-copy withdrawal. Fixture success is
not evidence of live provider qualification.
