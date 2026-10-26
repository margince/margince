<!-- describes: backend/internal/shared/kernel/langcopy/langcopy.go backend/internal/shared/ports/baselanguage/baselanguage.go backend/internal/modules/identity/baselanguage.go backend/internal/shared/kernel/dealrole/words.go backend/internal/shared/kernel/owedwork/duecopy.go -->
# Language in shared explanations

A shared record needs a stable explanation. A reader changing their interface language must not
change an approval's evidence, effect or dismissal identity.

## Shared prose and interface captions

Stored summaries use the installation's base language when written. Existing summaries keep their
original words when the setting changes. A fresh deterministic projection, such as an attention
card, uses the current base language. The projection resolves that setting once so its sections
agree even when an administrator changes the setting during the read.

[BaseLanguageForRecord](../../backend/internal/modules/identity/baselanguage.go) resolves the setting
inside an existing transaction. Its established fallback is English, with a logged error when the
setting cannot be read. Modules use the
[base-language port](../../backend/internal/shared/ports/baselanguage/baselanguage.go) when they need
an injected resolver.

A measured team weekly review stores its summary language. A read that withholds forecast figures
rebuilds the summary in that saved language. Older rows without language metadata use English;
an unmeasured snapshot carries no language until measurement replaces it.

A meeting brief resolves its language in a separate transaction before assembling the record.
The same value controls its deterministic prose, model requests and manager coaching. A failed
settings query can fall back to English without aborting the transaction that reads the meeting.
Its dates use ISO calendar dates; meeting times retain their timezone.

A report evaluation stores its explanation language in its context. Frozen projections reuse it;
legacy contexts use English. Comparisons explain a refusal in the selected right-hand edition's
language. The language setting does not change comparability, amounts, attribution or evidence keys.
Forecast capture failures retain the language used by the capture writer and their original retry time.

Account scan and rule receipts share source captions. The caption can change language while the
quoted message and dismissal identity stay the same. Budget and provider deferrals remain distinct
machine codes; the interface explains the known code and uses neutral wording for an unknown one.

Voice and website deferrals expose `deferral_reason` from the same persisted distinction used by
the retry scheduler. The interface translates that code. The legacy `status_detail` remains intact
because older workers use its provider marker; unknown response codes keep that original guidance.

Typed reason codes use the reader's interface language. For example, the blocked-domain contract
carries a reason code beside its compatibility sentence. The frontend renders the code; an unknown
code or missing parameters leaves the stored sentence readable. A human's note remains their words.
Voice review reasons store a code, parameters and compatibility text together. The persistence writer
creates both representations from the same reasons. Unknown codes, incomplete parameters or mismatched
metadata retain the original explanation.

## Keep evidence separate

A quotation, task subject or recorded name remains verbatim. Translate the sentence around it,
using a complete [Phrase](../../backend/internal/shared/kernel/langcopy/langcopy.go) for each language.
Translate complete sentences. Formatted phrases retain the argument order and types of the English
phrase; the placeholder census does not support indexed format verbs.

A caption cannot decide what happened. Domain admission writers store reason codes with provenance
that ties them to the decision. Readers ignore a code left behind by an older writer that replaced
that decision. The audit before-image uses the same check as the displayed record.

Evidence IDs, claim keys and dismissal fingerprints do not depend on translated captions.
Generated task subjects may use the base language, while the operation and target records stay the same. The contact and company cards share deadline wording through
[owedwork](../../backend/internal/shared/kernel/owedwork/duecopy.go). Recorded buying roles share
[dealrole](../../backend/internal/shared/kernel/dealrole/words.go).

## Prove both wording and behavior

Use [langcopytest.Census](../../backend/internal/shared/kernel/langcopy/langcopytest/census.go) to
check every phrase and its placeholders. Check singular forms separately. Exercise the real writer
or projection in each supported language, preserving quotations and decision facts. A dismissal
made before a language change must still hold after it; undo must restore the same underlying work.
