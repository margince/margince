# API compatibility and deprecation

Margince versions the HTTP API through its path prefix, such as `/v1`.
The public contract is [crm.yaml](../../backend/api/crm.yaml). Generated Go and
TypeScript types follow that contract.

## Changes a client must handle

A change breaks compatibility when a request or response that worked before
cannot be used in the same way. Examples include removing or renaming an operation
or field, changing a field's type, making an optional request field required,
removing an accepted request value, or tightening a validation rule.

Adding an optional response field or a new operation does not remove existing
behaviour. Clients should ignore unknown response fields. The compatibility gate
still checks changes to response enums because a strict generated client may
reject a new value.

[check-contract-breaking.sh](../../scripts/check-contract-breaking.sh) checks the
contract against its merge base. An approved compatibility exception does not skip the operation notice
and removal checks.

## The notice window

A deprecated API stays supported for at least 3 calendar months from the release
that announces its deprecation. The clock starts at that release's publication
instant. It starts neither at the implementation commit nor when an installation
upgrades. Dates use UTC. A month that lacks the starting day ends on its last day.

Patch releases never remove an API. A removal needs a known stable non-patch
release version, the full notice window, and any later published sunset date.
An unknown release format cannot authorize removal. The ordinary compatibility
gate must also approve the change; the end of the window does not remove an API
or grant an exception by itself.

The current CI lane supplies no removal release version, and the current release
format cannot authorize removal. Whole operations therefore stay supported. A
supported non-patch release lane must be added before retiring an operation.

A maintainer may ratify removing a named operation without a notice, for an
operation no stable release ever served. Each one is a line in
[contract-ratified-removals.txt](../../scripts/contract-ratified-removals.txt)
with its method, path and reason. The notice check and the compatibility check
both let that one operation go. Every operation not on the list keeps the rules
above.

The shared rules are in
[notice.go](../../backend/internal/platform/apideprecation/notice.go).
The check refuses a removal without a recorded announcement and prevents changes
that backdate a published notice or bring its sunset forward.

## Mark an operation

Set `deprecated: true` on the operation in `crm.yaml`. Until its announcing release
is published, leave `x-deprecation` absent. This keeps the operation callable and
publishes a `Link` header pointing to the API document and this policy. The absence
of a publication date gives no permission to remove the operation.

Record the actual announcing release and publication time in `x-deprecation`.
An optional `sunset_at` names a planned retirement date. This is an example of the
format, not a record of a release:

```yaml
deprecated: true
x-deprecation:
  release: v1.0.0
  announced_at: '2026-01-31T12:00:00Z'
  sunset_at: '2026-04-30T12:00:00Z'
```

A dated notice adds `Deprecation` as a structured Unix date. A planned retirement
adds `Sunset` as an HTTP date. The formats follow
[RFC 9745](https://www.rfc-editor.org/rfc/rfc9745.html) and
[RFC 8594](https://www.rfc-editor.org/rfc/rfc8594.html). They do not change the
operation's behaviour. The generated route notices also cover parameter errors
and refusals from operation middleware.

Announce the deprecated operation and its replacement in the release notes.
Keep its published release and publication date in the contract while it remains
deprecated. Withdrawing deprecation keeps the operation supported; any later
deprecation starts with a new release notice. Regenerate the
contract outputs with `make gen` after changing a notice.

Deprecated fields, parameters and enum values also keep the notice window and
patch-release rule. Their markers remain in the contract. The normal
compatibility check refuses breaking changes. Its explicit pre-live and exact
compatibility exceptions can bypass that comparison; this operation notice gate
does not track dates for individual fields, parameters or enum values. Those
exceptions must not be used to remove published fields during their notice window.

The API document includes a generated summary of every deprecated operation,
including operations the agent-specific contract does not otherwise expose.
The JSON document requires sign-in. The second policy link points to this public
page. Refusals made before routing, such as a session check, carry no operation
notice because no operation has been matched yet.
