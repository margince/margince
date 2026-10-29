# Implement deal viability review

Status: reviewed and reconciled implementation plan. The first slice is built: the pure assessment
(`deals.AssessViability`), the `deal_viability` rule with a receipt per deal, one task per review
episode, the qualified readings behind `assurance.qualified_forecast_enforced`, and the sweep's
category notch held while enforcement is on. The rest is open in #9487 to #9492.
Source baseline: `3f9a9d7d6196c6ab8e1d3f5bfb4accff15cb7414`.
The Claude review used `5daf71807da3e2e719e4aa4dd0d831f07ebb8888`; section 11 records its dispositions.
Section 12 records the inline outcome checks against the source baseline.

Deliver the connected workflow in one PR, using the work packages below.
Keep model training, outbound messaging, new probability scoring, and event-driven rescoring outside the initial delivery.

## 1. Set the business behavior

Assess customer silence independently of the expected close date.
Use a fixed-clock synthetic deal with no customer progress for 196 days and a recorded close date 94 days ahead.
Assert against the forecast period containing that date; a current-quarter assertion could pass without exercising the defect.
With enforcement active and source coverage sufficient, the deal has zero qualified forecast contribution and one persistent review task.
Include a Commit variant with positive baseline evidence, plus eligible and won controls; zeroing every forecast value must fail the test.

Keep its stage, baseline probability, nominal value, and recorded close date with their provenance.
Show customer silence, date credibility, and effective forecast eligibility separately.
An unsupported future date needs review. Replace it only with attributable evidence or a supported provisional estimate.
An unset provisional flag alone cannot prove customer confirmation; credibility needs attributable evidence or a current authorized attestation.
Never move a date further away merely to keep an unsupported opportunity open.

Use `StalledThresholdDays` from [formulas.go](../../backend/internal/modules/deals/formulas.go) as the initial customer-silence threshold.
Use a strict after-threshold boundary and the existing occurrence-precision rules.
Keep the existing stalled label and `IsStalled` unchanged: their general-activity clock answers a different question.
Keep short follow-up windows distinct. Replace assurance's separate `buyer_silent` decision with the shared viability result.

| Evidence | Viability decision |
|---|---|
| Supported progress and credible date | Eligible, unless another unresolved review reason applies. |
| Customer silence beyond the threshold | Review required, independently of close-date distance. |
| Recent unanswered outbound mail, field edit, task, or system correction | Cannot reset customer progress or resolve its review. |
| Customer-requested wait or confirmed future meeting | Apply the supported deferral and its expiry; still assess date credibility. |
| Required coverage unavailable, stale, or inaccessible | Unknown, with a visible coverage reason; never claim observed customer silence. |
| Won, lost, or archived | Close the applicable finding and reconcile its bundled work. |

For never-contacted deals, use the recorded commercial creation date after the threshold, only when source coverage establishes the absence.
An outbound attempt cannot move that anchor forward. Preserve the source creation date during an import.
Historical imports retain their original occurrence dates. Import arrival cannot become fresh customer progress or a new reply obligation.
An unrelated reply, automated reply, or canceled meeting cannot revive a deal.
Completing a task alone cannot restore forecast eligibility.
Once review opens, retain a requalification-pending reason until an authorized outcome validates the next step and date.
A reply can clear customer silence without closing that review. A negative or ambiguous reply cannot establish commercial progress by itself.
Keep assessment, review disposition, and effective eligibility separate: waiting or task completion is not evidence of forecast support.

## 2. Choose the owner and reuse points

Assurance is the sole producer and owner of persistent viability findings.
Compose supplies evidence to one pure deals assessment and passes its result on `assurance.Subject`.
One assurance rule consumes that result. The date corrector never produces a second viability finding.

| Capability | Source to extend |
|---|---|
| Customer and conversation evidence | [Engagement](../../backend/internal/modules/deals/engagement.go), [email move](../../backend/internal/modules/activities/emailmove.go), and the awaiting-answer predicate it uses in [awaiting reply](../../backend/internal/modules/activities/awaitingreply.go). |
| Pure viability and date facts | [Deal formulas](../../backend/internal/modules/deals/formulas.go) and [date assessment](../../backend/internal/modules/deals/closedate.go). |
| Findings, coverage, and resolutions | [Assurance scan](../../backend/internal/modules/assurance/scan.go), [rules](../../backend/internal/modules/assurance/rules.go), [store](../../backend/internal/modules/assurance/store.go), and [resolution](../../backend/internal/modules/assurance/resolve.go). |
| Persistent ordinary review tasks | [Bundle store](../../backend/internal/modules/assurance/bundle.go), [bundle composition](../../backend/internal/compose/assurancebundle.go), and [task adapter](../../backend/internal/compose/assurancebundletask.go). |
| Monetary contributions and landing | [Forecast readings](../../backend/internal/modules/forecasting/readings.go), [landing](../../backend/internal/modules/forecasting/landing.go), [forecast seam](../../backend/internal/compose/forecastseam.go), and the per-deal weight in [weighted value](../../backend/internal/shared/kernel/values/weighted.go). |
| Bounded run design | [Close-date membership](../../backend/internal/modules/deals/closedaterun.go), as an existing design to adapt within assurance's ownership. |
| Category notch and its policy | [Close-date correction](../../backend/internal/modules/deals/closedatecorrect.go), through the `CorrectionPolicy` seam that [compose](../../backend/internal/compose/closedate.go) fills. |

Keep modules within their table ownership. Compose connects them through declared transaction-aware seams.
Reuse existing weighted-value arithmetic, activities writers, source identity locks, outbox, River, and migration/data-task infrastructure.
Health scores remain advisory; they cannot introduce another automatic exclusion policy.

## 3. Build evidence and coverage explicitly

Pass one persisted assessment time and installation timezone through the scanner, subjects reader, SQL predicates, and pure assessment.
Replace wall-clock reads on this path with the supplied time. Preserve original occurrence precision and reject future evidence.

Read deal-scoped `activity_link` evidence, customer progress, latest relevant conversation, supported waiting, meetings, date provenance, and owner.
Reuse the existing human-writer, machine-sender, calendar-copy, canceled-meeting, and thread-disposition predicates.
Keep historical evidence distinct from current reply-obligation admission using the existing imported-history rules.

Do not call the personal email-state reader under a system principal.
Extend its underlying awaiting-answer predicate with an explicit sender set covering the relevant seats and a deal/counterparty scope.
The same predicate must serve personal and scheduled reads without copying their conversation logic.
A reply on another deal at the same company cannot settle this deal's obligation.
Internal correspondence cannot establish customer progress. A future meeting needs external confirmation before it can defer review.

Return typed eligible, review-required, waiting, and unknown outcomes, with reasons, evidence references, cutoff, and policy version.
Pass the current review disposition into the pure assessment through compose; keep assurance types outside the deals module.
Persist a receipt for every inspected deal, including eligible results without a finding and explicit unknown or failed outcomes.
Record the input revision, lifecycle revision, policy version, cutoff, coverage, and next time-based transition on that receipt.
The rule's required coverage must gate raising an observed-silence finding as well as clearing one.
The existing scan gates clearing only, so declaring `Needs` alone is insufficient.

Initially, reuse installation-wide mail coverage. Any required mailbox failure makes the result unknown and prevents a new exclusion.
A stale finding cannot remain exclusion evidence while its required source is unknown.
Report the affected population and permitted monetary subtotal separately. Per-seat coverage is a later enhancement.
Store reason codes and structured evidence, not rendered private correspondence.
Retire `buyer_silent` from active rule evaluation while retaining its stored type for history and older readers.
Reconcile its open findings into the viability episode only after a supported assessment; unknown coverage cannot close the legacy finding as cleared.

## 4. Make the assurance pass resumable

Extend the assurance run instead of placing finding writes in the close-date sweep.
The current assurance scan reads every deal in one transaction; the current `CloseCleared` assumes that complete walk.
A page must never clear findings merely because their subjects are outside that page.

Add assurance-owned durable membership and progress to the existing run model through additive migrations.
Freeze all live open deals independently of close date, with a persisted assessment time and complete denominator.
Keep one active assurance pass per installation; a concurrent manual recheck joins or resumes that pass.
Use indexed paging, bounded evidence reads, and per-member outcomes. Advance over every inspected member, including unchanged records.
Commit page results, findings, run-finding membership, audit, and delivery obligations through their existing owners.
Keep this member receipt available to qualified readers; absence of a finding cannot stand in for a successful assessment.

Clear an absent condition only after its subject was assessed with the required coverage.
Global absence-based clearing waits for a complete membership census and is restricted to that run's members and evidence cutoff.
Never clear an unvisited, failed, newly created, or coverage-unknown subject.
A closed or archived subject receives its explicit departed outcome.
Preserve existing signal checks and clearing under their own complete census; the deal paging change cannot clear unseen signals.

Re-read source revisions, current owner, status, and disposition before applying a result.
Skip and requeue obsolete evidence rather than committing a result over a newer reply or human resolution.
Settle an obsolete member as unassessed and enqueue a fresh assessment after the frozen pass; do not retry forever against its old cutoff.
An inspected member and a successfully assessed member have separate counts. Completing the walk cannot imply complete qualification.
Resume after the declared worker deadline. Persist incomplete runs and isolated failures instead of rolling the run record away.
Large-backlog tests must reconcile all members across retries, not merely finish one convenient fixture inside the deadline.

## 5. Extend review episodes and resolutions

Keep the existing `type:subject` finding identity and append-only resolution history.
For viability findings, add an episode generation and structured resolution basis within the existing assurance lifecycle.
Increment generation on a terminal-to-open transition after expiry, invalidated resolution evidence, renewed conditions, or reopening the deal.
Include `resolved`, `condition_cleared`, and `subject_departed`; retries of an already open generation cannot increment it again.
Preserve the generation while a finding remains open, including through rule changes, date edits, and reassignment.

Store qualifying-progress references and the relevant forecast claim revision in the resolution basis.
Do not use advancing silence-day counts, run dates, outbound mail, or machine date corrections as new episode identities.
A different close date alone cannot mint a new task for an already unresolved episode.
Extend viability upsert to refresh the open finding's owner and evaluate resolution expiry.
Limit reopening semantics to declared review rules; preserve unrelated assurance resolutions.

| Review action | Existing operation and checks |
|---|---|
| Revive | `fixed_record`, after server-side validation of supporting evidence, a next step, and a credible close date. |
| Wait | Authorized deals writer records the supported customer wait; `remind_later` retains the open review and schedules its return. |
| Override | `value_correct`, requiring reason and expiry bounded by the existing `MaxSuppression` rule. |
| Close lost | Existing deal-close writer and reason, followed by assurance's departed-subject reconciliation. |

Bound the review's wait action by the same suppression ceiling; do not globally restrict the existing `wait_until` API.
A longer customer wait may remain recorded, while this review asks for confirmation earlier.
Wait retains the episode and reschedules the same task to its return date. It cannot remove unsupported-date or missing-next-step reasons.
At `remind_at`, resume that task and review without creating another episode merely because time passed.
A wait alone cannot restore support. Restoration requires validated requalification or an authorized override covering all remaining viability reasons.
Expiry triggers the next scheduled or manual recheck. Live qualified reads treat an expired resolution as pending assessment, never as a permanent override.
A generic completion of the task leaves the review unresolved and visible; it cannot substitute for these outcomes.
Coordinate record edits and review resolution through version-checked compose actions and the owning modules' transaction-aware writers.
If a separate lifecycle event completes the review later, retain its durable reconciliation obligation and show the pending state.

## 6. Serialize task delivery and reuse

Extend `AssuranceTaskSource` bundling; compatibility means that source only. Ordinary follow-ups stay separate.
Within one transaction, take the existing write-identity lock for the source and deal before looking up or minting work.
Use an unrestricted system-task lookup for deduplication; an owner-visible lookup can miss an existing task.
Recheck current assignment and audience through activities writers before changing either.

For a viability episode, key the task by deal, stable viability exception id, and episode generation, never by run or cycle.
Persist its authoritative task link on the assurance-owned episode, including when adopting a legacy run-keyed task.
Look up this link before searching for open tasks. Keep it when the task is completed, archived, or no longer admitted by a task-age filter.
Use that canonical finding even when other bundled reasons change order. Other reasons adopt the same open assurance task.
A completed task with that identity remains delivered; it is not minted again while the episode is unchanged.
An unresolved review remains on the deal and Home even if someone completed its task.
An open obligation cannot expire through generic automatic-task recency cleanup; its source-specific review lifecycle decides whether it remains current.
Retain adoption and guarded cleanup of old duplicate system tasks, without overwriting human edits.
For a new task, set its initial deadline to the next installation calendar date after successful creation, as the proposed delivery default.
Use delivery time rather than the frozen scan cutoff, so a resumed backlog cannot create a task already overdue.
Store it once and retain it through retries and adoption. Explicit Wait or human rescheduling can change it through the existing task writer.

Refresh finding ownership from the live deal. Reassign an adopted task only if its assignee still matches the prior system assignment.
Preserve a human-selected assignee. Use the existing unassigned queue when the current owner cannot hold work.
Check the assignee's ability to read and resolve this review through the existing assurance gates, including `forecast:update` and deal write scope.
If authority is missing, retain actionable unassigned work and a visible handoff reason instead of assigning an unresolvable task or widening grants.
Use the transaction/savepoint fallback already established for invalid assignees; never grant access to make assignment succeed.
Task text carries generic reason codes and counts. Resolve names and evidence on read under the reader's current grants.

Commit a durable River delivery job or equivalent existing outbox obligation with the scan's findings.
A delivery retry uses the saved run and episode; it must not start a new scan to recover a missing task.
Declare any new job in the existing job catalog and use its normal retry and failed-job surfaces.
Report pending, failed, and delivered separately. A linked completed task counts as delivered, even if its review remains open.

After all findings on a bundled system task are resolved or cleared, complete it through a narrowly scoped activities writer.
Close only the identified assurance tasks. A reply cannot complete an unrelated human task or a bundle with another open reason.

## 7. Define forecast surfaces and compatibility

Exclusion is derived from a current, open, unsuppressed viability finding with sufficient coverage and active enforcement.
Eligibility also requires a current per-deal assessment receipt and a satisfied review disposition; no finding alone cannot prove eligibility.
New, unvisited, failed, obsolete, or policy-mismatched members are unknown even when another run completed for the installation.
It does not write a deal probability or category. Manual date and stage autonomy remain unchanged.
Add eligibility and reason fields distinct from the existing monetary `ExclusionReason`, which covers missing price and exchange rates.
Use the masked converted amount from the forecast seam for every new aggregate; never sum raw finding amounts.

Calculate the baseline weight once, then apply eligibility to qualified contributions.
Retain the old API readings and their meanings. Add qualified evidence, best-case, and weighted readings, plus excluded and unknown-coverage subtotals.
Include completeness and applicability states. An unknown amount or unknown coverage cannot masquerade as a zero-valued exclusion.

| Surface | Planned treatment |
|---|---|
| Deal details and Home | Show baseline stage probability, effective eligibility, reason, date credibility, and the persistent review. |
| Forecast API and MCP | Keep baseline readings; expose qualified readings and per-deal eligibility separately. |
| Default forecast landing | With enforcement active, the new UI explicitly requests `qualified_commit_evidence`, preserving the current evidence-first policy. |
| Optional weighted landing | Add `qualified_weighted`; keep the existing `weighted` measure available and labeled as baseline. |
| Manager call | Preserve the human's authored call; show its gap to qualified support instead of rewriting it. |
| Weekly review, saved copies, shares, exports, and Explain | Carry the selected measure and row-level eligibility; recipient-visible totals are re-summed from permitted rows. |
| Company pipeline, hierarchy rollups, stage boards, and nominal reports | Keep nominal and stage-weighted semantics and label them. They cannot be presented as qualified active forecasts. |

Extend shared measure definitions, API enums, weekly-copy schemas, and generated clients together.
Server defaults for legacy requests remain compatible. A client lacking qualified fields shows unavailable qualification and a clearly labeled baseline.
Explicit saved baseline selections remain baseline; do not relabel their values as qualified.
When enrollment, enforcement, or timely coverage is absent, qualified totals are unavailable or partial with that reason, rather than baseline disguised as qualified.
The worked fixture must show its unchanged baseline probability beside zero qualified contribution, an unsupported date, and the review-task link.
Any displayed effective probability is derived for the qualified forecast; it must not imply a calibrated zero chance of winning.
Fold equivalent risk, finding, and task rows by review identity on Home; keep distinct obligations separate.
Apply owner and visibility selection before limits, and derive Focus and summary counts from the resulting unique actions.

Freeze eligibility, policy version, cutoff, disposition, and completeness on each new contribution row.
Keep historical snapshots unchanged and mark qualified comparisons with older snapshots as definition changes.
Add excluded and reinstated movement causes and account for them in the weekly bridge's bucket census and presentation contract.
Do not misclassify eligibility changes as date pushes, amount changes, or unexplained model effects.

## 8. Cut over without category ratcheting

Reuse assurance enrollment and add one stored installation switch for qualified forecast enforcement, default off.
An enrolled installation can observe findings and tasks before enabling enforcement.
The read surfaces must say not enrolled, observing, enforcing, stale, or incomplete as applicable.
Do not report review-task delivery as an approval to change a deal field.

When enforcement is active, make the close-date corrector a date writer only and disable its repeated category notch.
Its future-date window can remain: assurance now assesses distant dates independently.
Compose supplies enforcement through the corrector's policy seam, with a fresh check before its write.
While enforcement is off, preserve the legacy correction behavior and label qualified forecasts unavailable.
Preserve categories already stored; never guess whether an old category came from a human or automation.
The existing date-correction autonomy and maintenance switch still govern date writes.

Use the existing assurance preview for the dry run and start flow.
Starting enforcement requires explicit installation authority and a current preview; the scan then reconciles existing records through the runtime path.
The preview binds its policy and configuration revision, evidence cutoff, population, and proposed actions; activation rechecks them or requests a fresh preview.
Reconcile applicable legacy `buyer_silent` resolutions and deferrals as well as open findings before applying the new rule.
Honor an unexpired decision whose basis still matches, retaining its actor and source reference without fabricating a new human resolution.
Preserve adopted task links and human edits during that reconciliation. An unsupported or expired legacy decision needs a stated reassessment reason.
Use additive migrations for run progress, episode state, delivery identities, and snapshot fields, and update the schema catalog.
Only legacy-row repair that cannot use that runtime path belongs in a bounded background data task.
Keep every shipped migration unchanged and preserve the declared supported upgrade path.

Stopping enforcement immediately removes the qualified application mode and exposes the labeled baseline, while retaining findings, tasks, and history.
Resolving or overriding a finding reverses its derived exclusion; do not build a separate field-undo engine.
Rollback cannot overwrite later human changes or delete ordinary work.
Update the handbook, API descriptions, and implementation explanations, including the conflicting close-date fallback descriptions.

## 9. Keep the first delivery bounded

Use scheduled assurance and the existing human recheck. Event-driven reassessment and per-seat coverage follow as separate enhancements.
Define one viability input revision, reusing existing source-basis fingerprint and transaction-hook patterns where their inputs match.
Invalidate it in the relevant write transaction, or compare batched current input fingerprints when reading; outbox lag alone cannot justify an old eligible result.
Include activity relinking, edits, archival, erasure, meeting, owner, date, lifecycle, and source-recovery changes.
This validation invalidates old receipts without running a second assessment engine or reusing a reader-specific status-card hash.
Derive the freshness deadline from the prior assessment time plus the job catalog's cadence and worker deadline.
Use that shared calculation in scheduled and interactive reads instead of adding another stored timing setting.
Also bound a receipt by its next threshold crossing, wait, meeting, or override expiry, whichever comes first.
When the bound is exceeded or a known source revision invalidates an assessment, qualified reads become stale or unknown immediately.
The UI may offer the existing recheck action. It must not advertise yesterday's result as current after the evidence changed.

Report full membership, assessed outcomes, unknown coverage, remaining members, and pending task delivery through the existing run surfaces.
Keep a failed subject from blocking unrelated subjects. Scheduled retries resume the saved run and outstanding deliveries.
Do not ship an unbounded one-transaction scan merely because a small benchmark passes.
Avoid a second review store, task type, money calculator, or personal-versus-worker conversation implementation.

## 10. Verify in the implementation PR

Build synthetic data through production writers and exercise the real worker, scanner, task adapter, and authenticated readers.
Inject the persisted assessment time into every relevant application and SQL boundary. Test database-time races separately with fixtures based on its clock.

- Prove the distant-date fixture in its actual forecast period, including the default qualified landing, best-case, weighted reading, and weekly copy.
- Assert positive baseline evidence on its Commit variant and unchanged eligible and won controls before checking the qualified exclusion.
- Cover strict threshold boundaries, date-only entries, timezones, never-contacted deals, and historically imported evidence.
- Send weekly unanswered outbound mail. Assert customer-silence review persists even when the separate stalled label clears.
- Cover unrelated replies on the same company, internal mail, machine senders, calendar copies, unconfirmed future meetings, canceled meetings, and supported waiting.
- Resolve and later detect a new episode; expire an override and wait. Assert one new generation and preserved resolution history.
- Reopen a lost or archived deal and renew a cleared condition. Assert the terminal finding starts a new generation and can obtain current review work.
- Record a reply without a credible date or next step. Assert silence clears while review, task, and zero qualified contribution remain.
- Complete a task with its finding still open. Assert no nightly replacement task and continued review visibility.
- Age an unresolved review past generic automatic-task windows. Assert its obligation remains current without a duplicate task.
- Clear one reason from a multi-reason bundle; close the deal. Assert task completion only when all its reasons allow it.
- Race a manual and scheduled scan, then concurrent delivery retries. Assert one active pass and one task for the episode.
- Adopt a legacy run-keyed task, complete or archive it, then retry delivery. Assert the persisted episode link prevents another task.
- Interrupt between finding and task delivery. Assert retry of the saved work, accurate pending state, and no rescan requirement.
- Change reply evidence, owner, date, authority, or status during assessment. Assert obsolete results cannot land.
- Cover owner inactivity and reassignment, preserve human task edits, and retain unrelated human tasks.
- Cover an owner lacking review authority and duplicate Home sources. Assert a visible handoff, one review action, and matching owner-scoped counts.
- Cover enrollment, observing and enforcing modes, date autonomy, source failure, stale results, and older client/server combinations.
- Add a deal after frozen membership and invalidate another receipt through relinking. Assert both read as unknown until individually assessed.
- Preserve an unexpired legacy override during cutover. At expiry, require reassessment rather than permanent suppression or immediate unsupported reinstatement.
- Verify amount masking on qualified, excluded, and unknown subtotals, row-level shares, exports, and MCP.
- Verify historical snapshots, cross-version definition movement, exclusion and reinstatement buckets, and bridge reconciliation.
- Exhaust a backlog beyond every page and batch bound across deadline retries. Ensure an unvisited page cannot clear a finding or signal.
- Exercise schema upgrades, preview, first-run reconciliation, restart, stop, and valid reversal through real writers.
- Exercise the full default-off activation flow on old synthetic deals. Assert the next pass produces the deadline, review link, and qualified exclusion without direct row edits.

Completion requires the rollout control to be exercised and its result verified; merging code with enforcement still disabled does not prove the intended outcome.

Run focused lanes during development, then the repository merge gate, touched database packages, and built-app coverage before shipping.
Run the requested code review workflow on the final implementation. This plan review does not substitute for code review.

## 11. Claude findings and reconciliation

The review identified four blocking design gaps. Their amendments below are part of the implementation contract.
The source claims were checked against the baseline above; migration constraints and default landing behavior were also inspected during reconciliation.

| Finding | Disposition in this plan |
|---|---|
| DVR-B1: no single pass owns findings and paging | Assurance is the sole producer. Extend its durable census and safe clearing in section 4. Reject the suggested unpaged benchmark-only shortcut because it leaves the supported backlog and deadline gap. |
| DVR-B2: resolved findings never recur; expiry is unused | Extend the existing lifecycle with episode generations, structured resolution basis, and expiry handling. Preserve unrelated resolution semantics. |
| DVR-B3: qualified weighted alone misses the headline | Qualify evidence and best-case too; keep evidence-first landing, explicit measure variants, weekly copies, shares, and compatibility in section 7. |
| DVR-B4: run-keyed tasks can duplicate concurrently | Serialize by source and deal, use stable viability episode identity, and transact task lookup, creation, and linking. Never key identity on whichever finding sorts first. |
| DVR-H1 and DVR-H2: stale owners and endless task re-asks | Refresh current ownership, preserve human assignment, retain delivered completion identity, and reconcile tasks only after every bundled reason closes. |
| DVR-H3: conflicting clocks and reader-specific mail state | Use one customer-silence policy, retain the separate stalled label, retire the competing buyer-silent rule through runtime reconciliation, and parameterize the shared conversation predicates. |
| DVR-H4: unknown coverage still raises silence | Gate the viability rule and qualified application on current sufficient coverage. Unknown remains visible without becoming an observed exclusion. |
| DVR-H5: autonomy is underspecified | Use derived forecast treatment under explicit installation enforcement. Preserve field-write autonomy and avoid restarting the approval-staging system. |
| DVR-H6: broadening date correction ratchets categories | Assurance closes the horizon gap; keep the date window and disable category notching when enforcement takes over. Preserve legacy behavior until activation. |
| DVR-M1 and DVR-M2: clock and movement gaps | Thread the persisted clock through SQL; add explicit eligibility movement and definition transitions for old snapshots. |
| DVR-M3 and DVR-M6: task and aggregate disclosure | Store generic task reasons, resolve private evidence on read, and derive every money subtotal from masked forecast rows. |
| DVR-M4: best-effort delivery loses work | Commit durable delivery with findings. Retry saved episodes without a rescan; completed linked tasks count as delivered. |
| DVR-M5: resolution outcomes form a closed contract | Map actions to existing resolution and deal operations, with server checks and bounded expiry. |
| Scope reductions | Reuse preview and runtime reconciliation; defer event-driven refresh and per-seat coverage; preserve nominal reports; avoid an exclusion-undo subsystem. |

Qualification is not claimed while disabled, unenrolled, stale, or unknown. The review's suggestion to return baseline as qualified in those states is rejected because it would hide the evidence gap.
The completed-task and stable-key recommendations are narrowed to preserve one task per unresolved deal review, including bundles with changing reasons.

## 12. Inline outcome reconciliation

| Gap | Required amendment |
|---|---|
| A reply could clear the whole viability finding | Retain requalification pending until the next step and date are validated. Wait defers the same review and cannot confer forecast support. |
| Only resolved findings advanced episode generation | Handle every terminal state, including cleared and departed findings, when its condition or deal returns. |
| Adopted tasks retained only their old run key | Keep an authoritative episode-to-task link through task completion, archival, age filters, and delivery retries. |
| No finding could be mistaken for eligibility | Require a current per-deal assessment receipt, including policy, coverage, revision, and time-based validity. |
| Deferred event-driven work left freshness undefined | Maintain evidence revisions through existing seams and bound receipt validity by known transitions, without adding a second scorer. |
| Cutover could discard existing decisions | Reconcile matching unexpired resolutions and task links before enforcement; bind and recheck the activation preview. |
| Default-off delivery could leave the original outcome unchanged | Verify the supported activation flow and the next scheduled pass, including an assigned or unassigned task with a stable deadline. |
| Assigned work could be impossible to resolve or appear twice | Check existing review authority, route blocked work visibly, and fold equivalent Home sources before counting. |
