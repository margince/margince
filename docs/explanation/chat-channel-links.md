<!-- describes: backend/internal/modules/chatlinks backend/internal/compose/chatsync backend/internal/compose/chatsummary backend/internal/compose/dispactapp backend/internal/platform/lastseen -->
# How chat channel links work

A channel association connects a company, deal or project to a provider destination. Attended
[Messenger](../handbook/messenger.md) is separate: it uses a member's personal authorization and
creates no automatic CRM transcript. Channel sync uses a shared bot connection, independently
controlled incoming recaps and outgoing CRM posts. The
[handbook](../handbook/chat-channels.md) describes the record controls; the
[provider contracts](../reference/messaging-provider-contracts.md) describe supported installations.

## Associations and handoff

The chat-link module owns association, root, delivery and member-consent records. Its generated
[entity model](../reference/entity-model/chatlinks.md) names the tables and their relationships.
Private message bodies are fetched on demand, rather than stored in these records.

A company may have its own channel before a deal exists. A deal can carry its association into a
project that has none. An existing project association takes precedence and is preserved. Company
History aggregates eligible child activities through the existing activity walk; it neither grants
child-record access nor creates a broadcast route from that aggregate.

A shared room requires an explicit root binding. An unrelated root or a mention of a record does
not establish one. Association identity includes the provider connection and immutable destination;
replacement associations cannot consume work claimed against an earlier link.

Unlink and archival retire association work. A surviving deal or project carrier can retain its
association through the existing lifecycle writer. Prior eligible recaps remain subject to CRM
visibility, provenance, restriction and retention. Ended sync records coverage actually observed;
it cannot describe provider changes after disconnection.

## From a CRM event to a provider post

The chat event consumer resolves record associations and records eligible events in the durable
queue. Event tiers control frequency; message detail separately controls summary length. The
[catalog](../../backend/internal/compose/chatsync/sync.go) and
[renderer](../../backend/internal/compose/chatsync/render.go) own those decisions.

A native delivery resolves the complete channel audience and current record readers before building
content. Every included activity must satisfy its content gate for that audience. Incomplete source
bodies stay pending with an explanation. The shared summary engine reads the complete bounded
source and preserves decisions, conditions and open questions. Source and audience are checked
again after model work. The outgoing preview uses this same engine without posting or changing the
queue.

Delivery claims commit before the provider call. An uncertain acknowledgement blocks later sends
for the association, even if a changed overview would otherwise produce a new request key. Receipts
record confirmed sends; an unknown result is not a delivered result. Explicit human review can stop
retrying uncertain updates. Restarting an uncertain overview requires a separate confirmation and a
new card generation.

When local restrictions withdraw content, successful provider rewrites are recorded separately from
unverified withdrawals. A removed connection or successful unlink does not prove provider erasure.
Incoming chat-origin notes are excluded from outgoing summaries to prevent a loop.

## From a provider thread to History

Signed provider notices carry scope and message identifiers, not a stored transcript. They queue
the same durable pull used by polling. Provider signatures, installation identity and replay checks
run before an event can affect a link. Teams also maintains metadata-only Graph subscriptions and
tracks bounded subscription leases for cleanup after disconnection.

The native root ledger separates scan progress from each root's processing state. A read must prove
a complete bounded thread before summarization. Missing pages and outages do not prove deletion.
Explicit tombstones can withdraw derived content. A post-model canonical reread prevents filing a
summary from a source that changed while the model ran.

A correction revises the existing recap under its stable source identity. Human edits are preserved
and marked for review when the source changes. Archiving does not allow the worker to resurrect the
recap. The activity write checks the live association and current authority in its transaction,
then commits the note, audit and outbox event together.

Legacy Dispact sequence-based episode processing remains available for existing associations. Its
cursor and posted card are preserved during upgrade. Canonical capabilities are verified before
using immutable IDs and complete-thread reads; missing capabilities are not inferred from a
successful legacy read.

Recap metadata exposes provider, recorded channel name, coverage and source-change markers.
Provider source bindings and URLs are withheld from ordinary activity projections. Opening a source
requires a fresh CRM read plus the connected member's current provider access. The returned proof
is short-lived and bound to that member and activity.

## Filing a chosen message

The channel-note operation accepts a provider-native message reference in the record's linked
channel. The source locator resolves its root; canonical reads prove both the selected message and
the complete bounded thread. Message-only and whole-thread selections use separate durable keys.
Repeated selection returns the existing note, while a source change or narrowed authority during
summarization refuses the write. This explicit operation does not enable recurring reads or backfill.

## The Margince app in Dispact

The `@Margince` agent in Dispact calls the same `/mcp` an MCP client calls, but with a different
credential. Dispact signs a short token for each call, naming the chat user who asked. The package
`compose/dispactapp` checks it in front of the passport path:

- The token must come from the connected bot's address, for its team, with a 60 to 120 second life and
  an id not seen before. The keys come from the address's published key set, through the same
  `oidctoken` verifier that checks Google and Microsoft sign-in tokens.
- The chat user is matched by their confirmed email to a live member, once. That member gets a passport of
  their own, and the passport keeps Dispact's issuer and subject (`passport.app_issuer`,
  `passport.app_subject`). Later calls find the passport by that pair. No other table holds the
  binding.
- So the agent never acts with more than that member could do in the app. The member's live grants and
  the passport's scopes decide every call, as for any agent. Revoking the passport ends it, and
  the revoked passport keeps refusing that chat user. Releasing it (`DELETE /passports/{id}/app-binding`,
  **Allow again** in Settings) clears the binding, so their next call mints a fresh passport.

Dispact reads the tool list once and shows it to every user, so the list cannot depend on who asked.
The edge gives the request the fixed list `dispactapp.Tools`. `tools/list` returns those tools and no
others, and `tools/call` refuses any other name as an unknown tool. A chat user who is no member may read
that list, and gets `403` `no_margince_seat` for anything else.
