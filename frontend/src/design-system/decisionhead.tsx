import type { ReactNode } from "react";
import {
  SummaryFields,
  type SummaryParts,
  type SummaryWords,
  summarySentence,
} from "./approvalsummary";
import {
  type DecisionDiff,
  type DecisionDisplay,
  type DecisionDraft,
  diffsOf,
  draftOf,
  type PayloadField,
  restFields,
  withoutRepeatedLead,
} from "./decisioncard.payload";
import type { DecisionApproval } from "./decisionstatus";
import {
  type ConfidenceLevel,
  ConfidenceMeter,
  type Provenance,
  ProvenanceTag,
} from "./trust";

/**
 * The words. Every one of them is the caller's, translated with `t()` at the
 * call site. A primitive that carried its own copy would be a second author of
 * the product's vocabulary. This one cannot know whether "Accept" releases an
 * email or promotes a lead.
 */
export type DecisionCardLabels = Readonly<{
  /** The three verdicts every decision surface offers. A verb is drawn only
   *  where its callback is given as well. */
  accept: string;
  edit: string;
  reject: string;
  /**
   * "Later". Optional because not every surface has this verdict. The inbox is
   * a queue somebody works to the end, and "later" there only moves work
   * sideways. Omitting it withholds the control even where `onSkip` is passed:
   * an unnamed button is a button nobody can act on.
   */
  skip?: string;
  /** What a lapsed card says in place of the verbs it can no longer offer. */
  expired: string;
  /** The drafted message's two halves, where a send-shaped payload carries them. */
  draftSubject: string;
  draftBody: string;
  /**
   * Opens and closes a clamped body. Both exist because the control's name
   * changes with its state. Optional as a pair, and only for a surface that
   * offers another way to the whole text. The inbox row's `aside` opens the full
   * payload one line above. A second affordance for the same words is two
   * answers to one question. Omit them and the body stays clamped.
   */
  showMore?: string;
  showLess?: string;
  /** What there is none of: a proposal whose payload carries nothing to read. */
  noContent: string;
  /** The labels of the fields a summary lists under its sentence. */
  summaryWords: SummaryWords;
  // What the card's body is waiting for, spoken while it waits. It sits beside
  // noContent because both answer one question for the two states of an absent
  // body. The card knows neither state; only the screen does.
  loading: string;
}>;

/**
 * The two words a compact row needs, and the switch that turns it on.
 *
 * Presence is the density: a surface that has the words has asked for the dense
 * form, and one that has not keeps the full row. Held as a pair rather than as a
 * boolean beside two optional strings. A compact row with no name for its fold
 * and its menu has its detail and two quieter verdicts out of reach. An
 * optional label on a control that must exist is a refusal waiting to happen.
 * That is why `Callout` carries its dismiss label with its handler.
 */
export type DecisionCompactWords = Readonly<{
  /** What opening the row shows: the subject, the message, the deadline. */
  detail: string;
  /** Names the menu holding the verdicts the line has no room to spell. */
  more: string;
}>;

// What the payload says, once, for the whole render. A reading rather than four
// derivations scattered through the JSX: whether the card has anything to show
// is the same question as what it shows. Answering it twice is how a card ends
// up drawing an empty state above its own content.
type PayloadReading = Readonly<{
  draft: DecisionDraft;
  diffs: readonly DecisionDiff[];
  /** The declared sentence that says why this is being asked, if the kind has one. */
  lead: string | null;
  rest: readonly PayloadField[];
  hasContent: boolean;
}>;

export function readPayload(
  approval: DecisionApproval,
  layout: "deck" | "row",
  declared: readonly DecisionDisplay[],
): PayloadReading {
  const change = approval.proposed_change ?? {};
  // A field the kind declares is drawn as declared. The draft reading must not
  // take it again as a drafted message.
  const owned = new Set(declared.map((entry) => entry.field));
  const read = draftOf(change);
  const draft = {
    subject: owned.has("subject") ? null : read.subject,
    body: owned.has("body") ? null : read.body,
  };
  // The headline and the summary are already on the card; a lead that only
  // repeats either is not drawn again.
  const display = withoutRepeatedLead(
    withoutRepeatedLead(declared, summarySentence(approval)),
    cardName(approval, draft),
  );
  const comparisons = display.flatMap((entry) =>
    entry.comparison
      ? [{ field: entry.field, label: entry.label, ...entry.comparison }]
      : [],
  );
  const diffs = display.some((entry) => entry.comparison !== undefined)
    ? comparisons
    : diffsOf(change);
  // The lead survives both layouts: the one sentence explaining why a decision
  // is in front of somebody is the thing a row is most missing.
  const lead = display.find((entry) => entry.lead)?.value ?? null;
  // Declared fields survive a row too, and only declared ones. The deck-only
  // rule was written against the raw reading. There a row unrolling nine wire
  // keys made a queue unreadable, which is why it stays for an undeclared kind.
  // A declared list is at most four short labelled rows. Suppressing them cost
  // the reader what they were asked about. The Worklist shows one decision at a
  // time, and its close-date card showed the reason with no proposed date. The
  // full declaration decides it, so dropping a repeated lead keeps a kind
  // declared rather than turning it raw.
  const rest =
    layout === "deck" || declared.length > 0
      ? restFields(change, draft, diffs, declared)
      : [];
  return {
    draft,
    diffs,
    lead,
    rest,
    hasContent:
      draft.subject !== null ||
      draft.body !== null ||
      lead !== null ||
      diffs.length > 0 ||
      rest.length > 0,
  };
}

// cardName is what this card is about, in the order a reader wants it. That is
// the drafted subject where there is one, then the record's own name.
//
// Spelled once because two places ask it: the headline and the article's
// accessible name. If the heading and aria-labelledby disagreed, screen
// readers would announce the card as something else.
export function cardName(
  approval: DecisionApproval,
  draft: DecisionDraft,
): string | null {
  return draft.subject ?? approval.target_label ?? null;
}

// The chips line, then the headline and the sentence explaining it.
//
// The headline is the drafted subject where there is one, because that is the
// line that differs. The server's summary names only the addressee. So a queue
// of drafts to the same few contacts reads as one sentence over and over.
//
// Failing that it is the record's name, where the server recorded one. Half the
// stageable kinds carry no typed payload. The summaries their paths compose
// name the target by uuid ("automation wants to assign_owner on deal
// 01a03781-9083-…"). A reader asked to approve that decides on the verb alone.
// The name answers "which record" in the one line they are certain to read.
//
// The summary then explains whichever of those two the headline was. It shows
// only underneath one: with neither, the summary is the headline, and printing
// it twice reads as two facts.
export function DecisionHead({
  approval,
  draft,
  labels,
  headingId,
  meta,
  aside,
  provenance,
  confidence,
  compact,
  listed,
}: Readonly<{
  approval: DecisionApproval;
  draft: DecisionDraft;
  labels: DecisionCardLabels;
  headingId: string;
  meta?: ReactNode;
  aside?: ReactNode;
  provenance?: Provenance;
  confidence?: ConfidenceLevel;
  /** One line: the question first, its qualifiers after it. */
  compact?: boolean;
  /** The summary's fields the card's body does not draw already. */
  listed?: SummaryParts;
}>) {
  const named = cardName(approval, draft);
  const sentence = summarySentence(approval);
  const headline = named ?? sentence;
  const chips = (
    <div className="dcard-meta">
      {meta}
      {provenance && <ProvenanceTag provenance={provenance} />}
      {confidence && <ConfidenceMeter level={confidence} />}
      {aside && <div className="dcard-aside">{aside}</div>}
    </div>
  );
  // A compact row is scanned by what is asked. So the question leads and the
  // chips qualify it. The tall card keeps the chips first. There the kind and
  // the countdown frame the rest of the card. It is a whole plate to read
  // rather than a line to scan.
  if (compact) {
    return (
      <>
        {headline && (
          <p className="t-body approval-headline" id={headingId}>
            {headline}
          </p>
        )}
        {chips}
      </>
    );
  }
  return (
    <>
      {chips}
      {/* What kind of line the headline is, where it is a drafted subject. It
          sits above the headline rather than beside a repeat of it further down:
          the subject printed twice on one card reads as two facts, and the
          reader then has to work out that they are the same string. */}
      {draft.subject && (
        <span className="t-eyebrow dcard-draft-label dcard-subject-label">
          {labels.draftSubject}
        </span>
      )}
      {headline && (
        <p className="t-h2 approval-headline" id={headingId}>
          {headline}
        </p>
      )}
      {named && sentence && <p className="approval-why">{sentence}</p>}
      <SummaryFields parts={listed} words={labels.summaryWords} />
    </>
  );
}
