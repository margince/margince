<!-- prose:plain -->
<!-- describes: backend/internal/modules/privacy/retentioncopies.go backend/internal/modules/webhooks/deliveryretention.go backend/internal/platform/database/storekit/outboxretention.go -->
# What an erasure leaves behind

An Article 17 erasure ([privacy engines](privacy-and-consent.md#the-privacy-engines-privacy))
removes the subject's personal values. These parts stay, each for its own reason:

- **Audit metadata.** Audit IDs, actors, actions and times stay. The images and evidence hold no
  personal value once they are cleared.
- **The suppression list** (`erasure_suppression`) and its copy in the object store under
  `erasure-suppression/`. Each entry is a hash of an address or a channel account, keyed by the
  [root key](../reference/storage-secrets-and-license.md#keys-derived-from-the-root-key). It stays
  so that the address is refused if it comes back.
- **The tombstone.** The contact row keeps its id and a stand-in name, so records that name other
  contacts still point at it.
- **Published outbox events** about other records stay for 30 days after they were published. The
  nightly pass then removes them. It keeps an event the deal check has not read yet (`assurance_inputs_read`). It
  keeps an event an imported lead's follow-up is told by, and one about a record under a hold or a
  retention floor. It never removes an event that is not published yet.
- **Webhook bodies.** A body about another record stays for 7 days after its delivery ended. A body
  still to be sent stays until it is sent or cancelled. The erasure clears the subject's own bodies at once.
- **Failed-login rows** in `system_log` keep a keyed hash of the address that was tried.
- **Backups** taken before the erasure hold the subject until they expire. A restore puts the
  rows back, and the suppression list in the object store erases them again
  (`ReapplySuppressions`).

The 30 and 7 days are defaults chosen for running Margince, not legal retention times. With
`privacy.retain_only` set, the nightly pass keeps both copies.

