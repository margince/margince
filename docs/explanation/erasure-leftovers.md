<!-- prose:plain -->
<!-- describes: backend/internal/modules/privacy/retentioncopies.go backend/internal/modules/webhooks/deliveryretention.go backend/internal/platform/database/storekit/outboxretention.go backend/internal/modules/privacy/erasure_manualreview.go backend/internal/modules/privacy/dedupeevidencescrub.go backend/internal/platform/database/storekit/auditredaction.go backend/internal/modules/privacy/linkedhistoricalaudit.go backend/internal/modules/privacy/pendingauditcopies.go -->
# What an erasure leaves behind

An Article 17 erasure ([privacy engines](privacy-and-consent.md#the-privacy-engines-privacy))
removes the subject's personal values. These parts stay, each for its own reason:

- **Audit metadata.** Audit IDs, actors, actions and times stay. Certified record images and
  ordinary evidence lose their personal values.
- **Controller decisions.** Reasons and the names of those who set or lift a hold or a limit on
  use stay in the audit trail. They record why content was kept or released.
- **Shared text.** Matches in company, deal and project text,
  mail signatures, signature templates and uploaded files are flagged for review. The
  original text stays until that review. Notes with no subject link need review when the name
  match is not clear or a hold keeps them.
- **Records under a hold or a retention floor.** Their personal values stay while the hold or
  floor applies. A limit on use does not prove that the values were erased.
- **The suppression list** (`erasure_suppression`) and its copy in the object store under
  `erasure-suppression/`. Each entry is a hash of an address or a channel account, keyed by the
  [root key](../reference/storage-secrets-and-license.md#keys-derived-from-the-root-key). It stays
  so that the address is refused if it comes back. When the subject confirms a link mailed to the
  address, the entry is lifted, not deleted (`lifted_at`). A later erasure arms it again.
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

Duplicate-match snapshots are cleared with the subject. This includes the `evidence` fields
inside old audit images. Audit IDs stay, and pending event copies linked to the cleared audit
rows are removed. Data tasks correct copies that older erasures left behind. Those
passes keep work written after the erasure and records that still need to be held.

The daily check returns to old erasure proofs whose audit copies still hold personal values.
It checks the hold again, so a hold that ends lets the next pass clear those copies.
Graph counts are set right in the erasure's own write, even when an earlier link change has
not yet sent its event.

The 30 and 7 days are defaults chosen for running Margince, not legal retention times. With
`privacy.retain_only` set, the nightly pass keeps both copies.

