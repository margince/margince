<!-- describes: backend/internal/shared/kernel/systemtext backend/internal/modules/identity/readerlanguage.go backend/internal/compose/readerrendering backend/internal/compose/companybrief/service.go backend/internal/shared/kernel/langcopy/langcopy.go backend/internal/shared/ports/baselanguage/baselanguage.go backend/internal/modules/identity/baselanguage.go backend/internal/shared/kernel/dealrole/words.go backend/internal/shared/kernel/owedwork/duecopy.go -->
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

## System wording for each reader

Task, activity, notice and approval writers can attach a private typed description of the product
wording. The existing record keeps its canonical text. Reads render a separate `system_display`
in the reader's language after checking access and content masking. Record IDs, proposal effects,
source evidence and dismissal keys continue to use the canonical record.

The shared [system text catalog](../../backend/internal/shared/kernel/systemtext) owns complete
English, German and Vietnamese phrases. Typed arguments distinguish source labels, dates, counts,
nested product phrases and ranges of quoted evidence. Display bounds apply in every shipped
language; only explicitly opaque labels may be shortened. An approval whose immutable facts cannot
fit retains its original summary and full preview, with a generic localized proposal caption.

Metadata binds each component to its canonical text digest. A real edit retires that component;
an unrelated update keeps it. Activity and notice triggers enforce this for direct SQL writers too.
Unknown codes, incompatible metadata and old rows without positive provenance show the original.
English-looking text alone never establishes that the product wrote it. Private descriptors are
not public API input and their arguments are not returned in display responses.

For a signed-in human, the validated `X-Margince-Display-Locale` header carries the effective UI
choice. A missing or invalid choice falls back through the saved reader language and installation
base language. An agent does not acquire a human display choice from this header. The browser
invalidates presentation reads when its locale changes; the Home cache separates locales.

Notices render again for their email or digest recipient, preserving the same notice and dedupe
identity. Public booking and record pages choose an explicit supported locale before browser
language preferences. Saved custom titles and quoted names remain their source text.

## Text for one reader

Two kinds of AI text get two languages:

- Shared or stored text uses the base language. An account summary, a relationship summary, a
  growth fit, a signal or a task subject is written once and read by everyone who can see the
  record. Writing it once per reader's language would multiply the model cost by the number of
  languages in use, and two colleagues would read two texts on one record.
- Text for one reader, stored for nobody else, uses that reader's language. The answer to a
  prepared question under **Ask about this company**, a document corpus answer and a plain-words
  filter's reasons go to the member who asked and are kept nowhere.

[ReaderLanguageForPrompt](../../backend/internal/modules/identity/readerlanguage.go) resolves the
reader's language: the display language the member chose, carried on their sign-in, else the base
language, else English when the setting cannot be read. `ReaderLanguageForRecord` answers the same
question inside a transaction. A system caller and an agent have no display language, so they get
the base language.

**Show in my language** on a task, note, mail, meeting or call is the same rule
([readerrendering](../../backend/internal/compose/readerrendering)). The press renders one text for
the reader who pressed it, and stores no copy, no audit row, no event and no AI payload. The button
also shows when the text's language is not known. When the text is already in the reader's
language, the answer says so and no model is called. A text longer than one model answer can carry
back is refused with that reason.

A task, note, meeting or call records its language when it is logged or its text is edited, read by
`textlang.OfMessage`, the function capture uses for mail. So a hand-logged German note shows the
button to an English reader and not to a German one.

Positively identified generated task prose can use that same reader-rendering lane automatically
when its row becomes visible and the server advertises availability. An ordinary GET never calls
a model. The request names one generated component and the expected source version; the service
checks access before sending it and reads the source again after the answer. Quoted components do
not enter an automatic request. The reader can keep or return to the original wording.

The browser keeps these renderings only in its volatile query cache, separated by reader, record,
version, locale and component. Reusing a cached rendering first checks current source access.
Entries expire after five minutes; the cache is bounded to 128 entries and two MiB, and at most two
requests run together. Failures retain the original and require an explicit retry. Sign-out,
reader changes and locale changes invalidate presentation state. Manual reading remains available
for older unclassified records; no model configuration leaves the original readable.

Shared company briefs, meeting briefs, weekly reviews and report editions retain their existing
stored-language rules. They need their own provenance and edition semantics before automatic
per-reader rendering can cover them.

## Keep evidence separate

A source quotation, human-written task subject or recorded name remains verbatim. Translate the sentence around it,
using a complete [Phrase](../../backend/internal/shared/kernel/langcopy/langcopy.go) for each language.
Translate complete sentences. Formatted phrases retain the argument order and types of the English
phrase. The placeholder census includes indexed verbs and verifies that translations keep their positions.

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

Booking profiles record whether the host chose the built-in title. A custom title remains custom
when it happens to match a translated default. The booking command and approval preview use the
same installation-language resolver for an omitted meeting subject.
