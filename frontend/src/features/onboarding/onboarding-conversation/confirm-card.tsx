import { Check, Sparkles } from "lucide-react";
import { useState } from "react";
import type { components } from "../../../api/schema";
import { Button } from "../../../design-system/atoms";
import { Heading } from "../../../design-system/heading";
import { formatNumber } from "../../../format/format";
import { useLocale, usePluralKey, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { coldFieldLabel } from "../../../lib/coldfieldlabels";
import type { CompanyDraft, CompanyFieldName } from "../onboarding";
import { REQUIRED_FIELDS } from "../onboarding";
import { useFactSelection } from "../onboarding-facts";
import { CoverageCard } from "../onboarding-live-panel";
import type { ClarifyAnswer } from "./company-proposal";
import { evidencedFields, isCompanyField } from "./company-proposal";
import {
  isWork,
  type ReviewGroupKey,
  reviewGroups,
  rowFor,
  STATE_RANK,
} from "./company-review-state";
import { CompanyIdentityCard, GroupBody } from "./confirm-card.groups";
import { SectionNav, useActiveSection } from "./confirm-card.nav";
import { CONTACTS_KEY, FACTS_KEY, type FrozenGroup } from "./confirm-card.row";
import { SaveRefusalLine } from "./save-refusal-line";

// The company review as a triage surface: every profile field is on the
// board, ordered so the work sits on top: required gaps first, weak evidence
// next, empty-but-optional after, and the solid rows last where a skim
// suffices. A company identity card leads the surface, a plain section list
// navigates it and names what's outstanding, and a progress bar pinned to
// the foot carries the one action that matters — no colour-coded tally of
// the board's own rows, which only ever repeated what the nav already says
// by name. Evidence-or-omit holds throughout: a grounded value keeps its
// verbatim snippet one interaction away, a human value says so, and an
// empty field is shown AS empty, fillable in place, rather than silently
// absent.

type Proposal = components["schemas"]["OnboardingCompanyProposal"];

export type CompanySiteRead = components["schemas"]["CompanySiteRead"];

export type SiteContact = components["schemas"]["CompanySiteReadContact"];

export type SiteFact = components["schemas"]["CompanySiteReadFact"];

// This card no longer re-asks a clarify of its own — that surface is the
// live DecisionScene's alone, and `open_questions` past the one currently
// live are promoted there in turn (use-company-read.ts) rather than shown
// here — so it carries no clarify-answering props at all: no comparisons to
// label a dismiss against, no pendingQuestionId, no answer/dismiss handlers.
export type CompanyConfirmCardProps = Readonly<{
  /** The installation's company once one exists; its mark keys on it. */
  companyId?: string;
  proposal: Proposal;
  draft: CompanyDraft;
  answers: readonly ClarifyAnswer[];
  /** The read behind the proposal; the card keeps its contacts and coverage
   * honesty reachable below the fields. Null on a proposal-only render. */
  read?: CompanySiteRead | null;
  selectedFactKeys: readonly string[];
  setSelectedFactKeys: (keys: string[]) => void;
  missingRequired: readonly CompanyFieldName[];
  setField: (field: CompanyFieldName, value: string) => void;
  onAcceptAll: () => void;
  pending: boolean;
  /** A clarify authorization is still in flight; accepting must wait for it. */
  authorizing: boolean;
  error: string | null;
}>;

// The form's words for the record's four groups; which field sits under
// which heading is `reviewGroups()`'s, shared with the whole-record article.
const GROUP_LABELS: Readonly<Record<ReviewGroupKey, MessageKey>> = {
  identity: "ob.s1.identityLabel",
  offer: "ob.s1.offerLabel",
  customer: "ob.s1.customerLabel",
  sales: "ob.s1.salesLabel",
};

// The one place the review is left from, pinned to the foot of the work
// surface so it survives however far the board is scrolled. It carries
// exactly three things: real progress toward being ABLE to continue (the
// required fields, not a tally of every row), the plain-words reason
// continuing is blocked when it is, and the one action that moves on —
// labelled Continue, a step transition, not "Accept all": pressing it does
// not bulk-apply the AI's proposed values (the human already settled each
// row by editing), it advances past a review that is done. That action is
// the same `onAcceptAll` prop the board has always had — there is no
// separate continue affordance to consolidate with, so this is a
// relocation, not a second control: the count that disables it is
// `missingRequired`, the exact predicate the section nav's blocking count
// already reads (`isRequired(field) && value === ""`), so the two can never
// disagree about how many gaps are left.
function ReviewContinueBar({
  filled,
  total,
  remaining,
  blockedByQuestion,
  pending,
  authorizing,
  onContinue,
  t,
}: Readonly<{
  filled: number;
  total: number;
  remaining: number;
  /** An open clarify question with no matching answer yet — the same
   * condition the live DecisionScene exists to resolve. Continue must gate
   * on this itself rather than trust that the scene is always what is on
   * screen instead of this card; that trust is exactly what left a one-
   * render window where neither did. */
  blockedByQuestion: boolean;
  pending: boolean;
  authorizing: boolean;
  onContinue: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const { locale } = useLocale();
  const keyFor = usePluralKey();
  const statusId = "ob-triage-continue-status";
  // Required fields first: it is the more actionable of the two blockers (a
  // value to type, right here) and the one this surface can always explain by
  // name elsewhere on the board; the open question is the narrower, rarer case.
  const statusKey =
    remaining > 0
      ? keyFor("ob.conv.review.requiredRemaining", remaining)
      : blockedByQuestion
        ? "ob.conv.review.confirmQuestionOpen"
        : "ob.conv.review.requiredDone";
  return (
    <div className="ob-triage-continue">
      <progress
        className="ob-triage-continue-bar"
        value={filled}
        max={total}
        aria-label={t("ob.conv.review.progressLabel")}
      />
      {/* A live region, not merely visible text: the reason named here is
          exactly what the button below is disabled on, so a screen reader
          hears why the moment it changes rather than only if focus happens
          to land on this paragraph first. */}
      <p
        id={statusId}
        className="ob-triage-continue-status t-sub"
        role="status"
      >
        {t(statusKey, { count: formatNumber(remaining, locale) })}
      </p>
      <Button
        variant="primary"
        aria-describedby={statusId}
        disabled={pending || authorizing || remaining > 0 || blockedByQuestion}
        onClick={onContinue}
      >
        {pending ? (
          <>
            <span className="ob-spinner" /> {t("ob.s1.saving")}
          </>
        ) : (
          <>
            <Check aria-hidden /> {t("ob.conv.review.continue")}
          </>
        )}
      </Button>
    </div>
  );
}

export function CompanyConfirmCard(props: CompanyConfirmCardProps) {
  const t = useT();
  const { locale } = useLocale();
  const byName = new Map(
    evidencedFields(props.proposal.fields)
      .filter((field) => isCompanyField(field.field, props.draft.values))
      .map((field) => [field.field, field]),
  );
  const groups = reviewGroups().map((group) => {
    const labelKey = GROUP_LABELS[group.key];
    const rows = group.fields
      .map((field) =>
        rowFor(
          field,
          props.draft,
          byName,
          t,
          props.read?.pages,
          props.read?.legal_entities,
        ),
      )
      .sort((a, b) => STATE_RANK[a.state] - STATE_RANK[b.state]);
    return { ...group, labelKey, rows };
  });
  const rows = groups.flatMap((group) => group.rows);
  const facts = props.proposal.facts ?? [];
  // The LAYOUT is frozen at mount — row order, where the solid rule falls,
  // which rows open by default, which of the two non-field sections exist
  // at all — while every row's state, colour and value stay live. Sorting
  // live instead would reshuffle the board under the cursor: typing one
  // character into an empty field re-ranks it settled and the row would
  // move mid-keystroke.
  const readAtMount = props.read;
  const factsAtMount = facts;
  const [frozen] = useState<readonly FrozenGroup[]>(() => {
    const fieldGroups = groups.map((group) => ({
      key: group.key,
      labelKey: group.labelKey,
      order: group.rows.map((row) => row.field),
      workCount: group.rows.filter((row) => isWork(row.state)).length,
      openByDefault: new Set(
        group.rows
          .filter((row) => STATE_RANK[row.state] <= STATE_RANK.empty)
          .map((row) => row.field),
      ),
    }));
    // Contacts join the board only once there is a read to report on: without
    // one, "no contacts found" would be a guess rather than a finding. They
    // carry no field order and no outstanding count — nothing here is the
    // human's to resolve, only theirs to see. Facts join once the read
    // actually produced any — an empty facts section would be nothing to
    // navigate to. Fields come first in the list on purpose: they are the
    // decisions the human owes, contacts and facts are what the read found —
    // last in the nav and last in the scroll order keeps the board's
    // weight on the work, however many facts the crawl turned up.
    const extra: FrozenGroup[] = [];
    if (readAtMount != null) {
      extra.push({
        key: CONTACTS_KEY,
        labelKey: "ob.conv.triage.contactsLabel" as const,
        order: [],
        workCount: 0,
        openByDefault: new Set<CompanyFieldName>(),
      });
    }
    if (factsAtMount.length > 0) {
      extra.push({
        key: FACTS_KEY,
        labelKey: "ob.conv.triage.factsLabel" as const,
        order: [],
        workCount: 0,
        openByDefault: new Set<CompanyFieldName>(),
      });
    }
    return [...fieldGroups, ...extra];
  });
  const activeSection = useActiveSection(frozen);
  const rowByField = new Map(rows.map((row) => [row.field, row]));
  // The contract ceiling on `selected_fact_keys` is the selection model's to
  // enforce, wherever a fact is picked: this card's toggles and the fact
  // table's checkboxes write the same key list, so they refuse on the same
  // terms.
  const factSelection = useFactSelection(
    facts,
    props.selectedFactKeys,
    props.setSelectedFactKeys,
  );
  // Questions the reader themselves declined are named honestly: nothing was
  // written, the field stays theirs to edit, never silently swallowed.
  //
  // Questions another answer RETIRED are not, and that exclusion is the point
  // rather than a tidy-up. Choosing a legal entity settles the sibling
  // questions about that entity's own address and registration and then fills
  // exactly those fields — so without the split the board printed "you
  // skipped: Registered address" directly under the address it had just read
  // from the legal notice, about a question the reader was never shown.
  const dismissedLabels = props.answers
    .filter(
      (answer) => answer.dismissed === true && answer.autoResolved !== true,
    )
    .map((answer) => coldFieldLabel(answer.field, t))
    .join(", ");
  // What the bottom bar's progress and gate read: real completion toward
  // being ABLE to continue, not a count of every field the board carries —
  // an optional field left empty or lightly grounded must never subtract
  // from a bar that claims to measure "can I move on".
  const requiredTotal = REQUIRED_FIELDS.length;
  const requiredRemaining = props.missingRequired.length;
  const requiredFilled = requiredTotal - requiredRemaining;
  // Confirm must not rely on the DecisionScene happening to occupy the
  // surface instead of this card — that is a layout accident, not a gate.
  // A question counts as unresolved exactly when the proposal still lists
  // it AND no answer has landed for its id yet; once every open question
  // has a matching answer (however it was answered), none of them block.
  const hasUnresolvedQuestion = (props.proposal.open_questions ?? []).some(
    (question) =>
      !props.answers.some((answer) => answer.clarifyId === question.id),
  );

  return (
    <section className="ob-conv-confirm ob-triage">
      <header>
        <Sparkles aria-hidden />
        <Heading size="large">{t("ob.conv.review.title")}</Heading>
      </header>
      <CompanyIdentityCard draft={props.draft} companyId={props.companyId} />
      <div className="ob-triage-body">
        <SectionNav
          groups={frozen}
          activeKey={activeSection}
          rowByField={rowByField}
          contactsCount={props.read?.contacts.length ?? null}
          factsCount={facts.length}
          t={t}
        />
        <div className="ob-triage-groups">
          {frozen.map((group) => (
            <GroupBody
              key={group.key}
              group={group}
              rowByField={rowByField}
              setField={props.setField}
              read={props.read ?? null}
              facts={facts}
              factSelection={factSelection}
              locale={locale}
              t={t}
            />
          ))}
        </div>
      </div>
      {dismissedLabels !== "" && (
        <div className="ob-conv-confirm-skipped">
          <p>{t("ob.conv.review.skipped", { fields: dismissedLabels })}</p>
        </div>
      )}
      {/* The read's remaining honesty: what it read or skipped. Contacts and
          facts both moved up onto the board as findings of their own — this
          is crawl provenance, not a finding, so it is the one thing left
          under a tail head of its own. */}
      {props.read != null && (
        <div className="ob-triage-readmore">
          <p className="ob-triage-rest-head t-eyebrow">
            {t("ob.conv.triage.restTitle")}
          </p>
          <CoverageCard
            pages={props.read.pages}
            warnings={props.read.warnings}
            stoppedReason={props.read.stopped_reason}
          />
        </div>
      )}
      <SaveRefusalLine message={props.error ?? null} />
      <ReviewContinueBar
        filled={requiredFilled}
        total={requiredTotal}
        remaining={requiredRemaining}
        blockedByQuestion={hasUnresolvedQuestion}
        pending={props.pending}
        authorizing={props.authorizing}
        onContinue={props.onAcceptAll}
        t={t}
      />
    </section>
  );
}
