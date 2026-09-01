import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { stable } from "../format/collate";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  ENGAGEMENT_LABELS,
  ENGAGEMENT_TONE,
  nextCommitmentLine,
  type SuggestionAction,
  useSuggestionsBody,
} from "./company360";
import {
  HEALTH_RATING_LABEL,
  HEALTH_STANDING_TONE,
  useAccountStanding,
} from "./companylookups";
import { EntityRef } from "./entityref";
import { CallCard, TodayPanel, TodoRow } from "./record360";

// "Today on this account" — the record's daily brief, and the only part of
// the page that answers *what do I do now*. It replaces two earlier cards
// ("Today on this account" and "Worth doing next") that used to say the same
// things twice between them: this one merged reading, not two that agree.
//
// TWO CARDS, both the record-360 kit's. THE CALL: the account's standing with
// whose move it is under it, and the thread it was read from. THE DAY'S WORK:
// the advice rows and the two verbs (draft a follow-up, prepare a meeting)
// that act on that context. The call states what IS; the work says what to DO
// about it, and the split between them is this component's whole reason to
// exist.
//
// The chrome is the kit's (`CallCard`, `TodayPanel`, `TodoRow`) so a contact
// and a deal read the same way; what this file owns is the ACCOUNT's answers —
// which standing, whose move, which rows.
//
// A reading that is BAD NEWS still says so, in colour: an account gone quiet,
// a deal stalled, a move that has been ours for weeks. Nothing else is
// coloured, so the colour means one thing.
//
// WHAT THIS DELIBERATELY DOES NOT CARRY, and the rule behind it: a second,
// weaker rendering of a claim that already has a good one is the duplication
// the page's own rules forbid.
//
//   - the open tasks THEMSELVES, which the Tasks screen lists in full with
//     their quick actions. The footer's commitment reading answers how many
//     are open and how soon, and never repeats a subject you would act on
//     somewhere better;
//   - a converted pipeline total or a KPI reading the strip already carries
//     for the account's STANDING state — this band carries what is DATED.

type Organization360 = components["schemas"]["Organization360"];

// The lead reading: whose move it is, under the call and in its own weight. It
// is the one thing a reader must know before the moves under it mean anything.
type TodayLead = {
  headline: string;
  // How long it has stood that way, beside the state and quieter than it. A
  // state with no duration is a status; with one it is a fact a rep can act
  // on.
  note?: string;
  // Colours the state where it is bad news — an account gone quiet, a move
  // that has been ours for weeks.
  tone?: "warn" | "danger";
};

export function TodayOnThisAccount({
  orgId,
  view,
  loading,
  failed,
  onPrepareMeeting,
  onDraftTo,
  onOpenRecord,
  onPerform,
  onOpenTasks,
  spine,
}: Readonly<{
  orgId: string;
  view?: Organization360;
  loading: boolean;
  // The composite read failed. Distinct from "still loading" and from "nothing
  // is happening on this account" — all three draw a short section, and only
  // one of them is a fact about the account.
  failed: boolean;
  // Opens the pre-meeting brief for the day's meeting.
  onPrepareMeeting?: (activityId: string) => void;
  // Starting a message from the account: opens the composer anchored on the
  // account and its recipient.
  onDraftTo?: (personId: string) => void;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Performing a suggestion's own action. The composer, the deal and the
  // task form all live above this brief.
  onPerform?: (action: SuggestionAction) => void;
  // Where the footer's commitment reading leads. Absent for a caller with no
  // Tasks tab of its own.
  onOpenTasks?: () => void;
  // The account's story as a thread, under the call it was read from. The two
  // are one statement about the account, which is why they share a card: the
  // verdict is the sentence and the thread is the evidence for it.
  spine?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Called before the loading/failed branches below, like every other hook
  // here: React requires it, and it answers "nothing rated" on its own.
  const verdict = useAccountStanding(orgId, view?.health);
  const suggestions = useSuggestionsBody({
    orgId,
    view,
    onOpenRecord,
    onPerform,
  });

  // Neither the pending read nor the failed one draws the call card. That
  // card exists to state a verdict, and a card holding a spinner where the
  // verdict goes is the reading claiming to have reached one. The day's work
  // still says which of its states it is in, under its own name.
  if (loading) {
    return <TodayPanel state="loading" onOpenTasks={onOpenTasks} />;
  }
  if (failed || !view) {
    return <TodayPanel state="failed" onOpenTasks={onOpenTasks} />;
  }

  const lead = whoseMove({ view, t, locale });
  const manualMoves = manualMoveRows({ view, t, onPrepareMeeting, onDraftTo });
  const commitment = nextCommitmentLine(view, locale, t);
  // The same verdict the readings row's health card states, off the one hook
  // that owns it — two computations of "how is this account" would agree until
  // one of them learned about payment and the other did not.
  // Nothing rated is itself a standing, and it says so: an account too new to
  // score reads "not assessed" rather than dropping the call and leaving the
  // sentence under it with nothing to belong to. The words are the readings
  // row's own for the same state, so the two cannot disagree.
  const standing = verdict.overall
    ? {
        label: t(HEALTH_RATING_LABEL[verdict.overall]),
        tone: HEALTH_STANDING_TONE[verdict.overall],
      }
    : { label: t("co.strip.notAssessed"), tone: "unknown" as const };

  return (
    <>
      <CallCard
        name={view.organization?.display_name}
        standing={standing}
        because={lead ? leadSentence(lead) : undefined}
        restsOn={verdict.restsOn}
      >
        {spine}
      </CallCard>
      <TodayPanel
        onOpenTasks={onOpenTasks}
        footer={briefFooter(commitment, suggestions.footer)}
        notice={
          (view.sections_omitted?.length ?? 0) > 0 ? (
            <TodayWithheld view={view} />
          ) : undefined
        }
      >
        {suggestions.rows}
        {manualMoves}
      </TodayPanel>
    </>
  );
}

// The band under the rows: what the brief as a whole is counting down to, and
// what the advice could not show. Undefined rather than an empty fragment when
// there is neither — Panel draws the band on truthiness, and an empty band is
// a row of the record spent on nothing.
//
// The way OUT of the brief is deliberately not here: a footer holding one link
// is that same wasted row, so "View tasks" sits beside the panel's name.
function briefFooter(
  commitment: ReturnType<typeof nextCommitmentLine>,
  fromAdvice: ReactNode,
): ReactNode | undefined {
  if (!commitment && !fromAdvice) {
    return undefined;
  }
  return (
    <>
      {commitment && (
        <Badge tone={commitment.overdue ? "warn" : undefined}>
          {commitment.headline}
        </Badge>
      )}
      {fromAdvice}
    </>
  );
}

// The verdict head's one line: whose move it is, and how long it has been that
// way.
//
// Two spans rather than one joined string, because they are two facts: a state
// with no duration is a status, and a rep cannot act on a status. Joined, the
// duration also stops being separately readable.
function leadSentence(lead: TodayLead): ReactNode {
  return (
    <>
      <span className="today-lead-state">{lead.headline}</span>
      {lead.note && <span className="today-lead-note">{lead.note}</span>}
    </>
  );
}

function manualMoveRows({
  view,
  t,
  onPrepareMeeting,
  onDraftTo,
}: Readonly<{
  view: Organization360;
  t: ReturnType<typeof useT>;
  onPrepareMeeting?: (activityId: string) => void;
  onDraftTo?: (personId: string) => void;
}>): ReactNode[] {
  const recipient = [...(view.people?.data ?? [])].sort(byStrengthThenId)[0];
  const meeting = view.next_meeting;
  const rows: ReactNode[] = [];
  // "Draft follow-up to <name>" names the strongest reachable contact,
  // because a row that says who it will write to is a decision the reader can
  // check before pressing it. It opens the composer grounded on the ACCOUNT
  // rather than on a message, which is why it passes the recipient.
  if (recipient && onDraftTo) {
    rows.push(
      <TodoRow
        key="move:draft"
        who={recipient.full_name}
        title={t("today.draft.to", { name: firstName(recipient.full_name) })}
        // Who the row would actually write to, under the ask. The title names
        // a first name because that is how the ask reads; a row a reader is
        // deciding whether to press has to say WHICH person that is, and two
        // Phans on one account is the ordinary case rather than the odd one.
        meta={recipient.full_name}
        verb={{
          label: t("today.draft.act"),
          onAct: () => onDraftTo(recipient.person_id),
          byMargince: true,
        }}
      />,
    );
  }
  if (meeting && onPrepareMeeting) {
    // Each attendee links to their own record: a rep preparing for a meeting
    // reads the guest list to decide who to look up, and a name they have to
    // retype into search is the one step this row exists to save.
    const who =
      meeting.participants.length > 0
        ? meeting.participants.map((participant, at) => (
            <span key={participant.person_id}>
              {at > 0 && ", "}
              <EntityRef
                kind="person"
                id={participant.person_id}
                name={participant.display_name}
              />
            </span>
          ))
        : undefined;
    rows.push(
      <TodoRow
        key="move:meeting"
        who={meeting.participants[0]?.display_name ?? meeting.subject}
        title={meeting.subject}
        meta={who}
        verb={{
          label: t("today.meeting.prepare"),
          onAct: () => onPrepareMeeting(meeting.activity_id),
          byMargince: true,
        }}
      />,
    );
  }
  return rows;
}

function firstName(fullName: string): string {
  return fullName.split(" ")[0] || fullName;
}

// omitted answers "was this withheld from me", which the page must never
// render as "there is none".
function omitted(view: Organization360, section: string): boolean {
  return (view.sections_omitted ?? []).some((name) => name === section);
}

// The sections this brief is assembled from. Naming them lets the footer say
// which ones the reader is missing, rather than silently composing a shorter
// brief and letting them believe it is complete. Each of the context band's
// several sections is withheld INDEPENDENTLY — one missing grant must not
// blank the rest of the band, and this list is what lets the footer say
// exactly which one it was.
//
// One entry per section a tile actually reads, and no others: a footer that
// reports a withheld section nothing here uses teaches the reader to ignore
// it, and one that omits a section a tile DOES use lets that tile vanish in
// silence.
const TODAY_SOURCES: ReadonlyArray<{ section: string; label: MessageKey }> = [
  { section: "next_steps", label: "today.source.nextSteps" },
  { section: "next_meeting", label: "today.source.nextMeeting" },
  { section: "people", label: "today.source.people" },
  { section: "deals", label: "today.source.deals" },
  // Whose move it is and the risk tile both read `state_strip` (whoseMove
  // and openRisk, below) — that is the section name the server actually
  // omits when a caller has no grant on it.
  { section: "state_strip", label: "today.source.standing" },
  { section: "activities", label: "today.source.activities" },
  // The moves themselves: a withheld advice section reads as "nothing to
  // add" from `useSuggestionsBody`, which is the right call for the rows —
  // but the footer still has to say the reader is missing them.
  { section: "suggestions", label: "today.source.suggestions" },
];

function TodayWithheld({ view }: Readonly<{ view: Organization360 }>) {
  const t = useT();
  const hidden = TODAY_SOURCES.filter((source) =>
    omitted(view, source.section),
  );
  if (hidden.length === 0) {
    return null;
  }
  // "Hidden from you", never "None". A brief assembled from some of its
  // sources is not the same brief, and the reader is the only one who can
  // judge whether the missing one mattered.
  return (
    <p className="today-withheld">
      {t("today.withheld", {
        sections: hidden.map((source) => t(source.label)).join(", "),
      })}
    </p>
  );
}

// Whose move it is, and how long it has been. Lifted from the state strip's
// own engagement tile rather than re-derived: the strip no longer draws it
// (it is a DATED reading, not the account's standing state), and the brief
// reads the same `state_strip.engagement` field the strip used to.
function whoseMove({
  view,
  t,
  locale,
}: Readonly<{
  view: Organization360;
  t: ReturnType<typeof useT>;
  locale: Locale;
}>): TodayLead | null {
  const engagement = view.state_strip?.engagement;
  if (!engagement) {
    return null;
  }
  return {
    headline: t(ENGAGEMENT_LABELS[engagement.state]),
    note: silenceNote(view, locale, t),
    tone: ENGAGEMENT_TONE[engagement.state],
  };
}

// How long the account has been silent, counted from the last thing WE sent
// and measured against the same `as_of` the rest of the page is read at — so
// the number does not drift while a tab is left open, and does not disagree
// with the dates beside it.
//
// Only when they owe us. Counting days since our own last message says
// nothing when the reply already came, and "no answer in 19 days" against a
// thread that ended with their answer is the kind of line that costs a reader
// trust in the whole panel.
function silenceNote(
  view: Organization360,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string | undefined {
  const engagement = view.state_strip?.engagement;
  const sent = engagement?.last_outbound_at;
  if (!sent || engagement.state !== "waiting_on_them") {
    return undefined;
  }
  const days = Math.floor(
    (Date.parse(view.as_of) - Date.parse(sent)) / MS_PER_DAY,
  );
  // Same day, or a clock that disagrees with itself: "no answer in 0 days"
  // reads as a fault, and there is nothing yet to report.
  return days > 0
    ? t("today.silence.days", { count: formatNumber(days, locale) })
    : undefined;
}

const MS_PER_DAY = 24 * 60 * 60 * 1000;

// Strongest first, with the id as the tiebreak so two contacts on the same
// score do not swap places between renders of the same data.
function byStrengthThenId(
  a: Organization360Contact,
  b: Organization360Contact,
): number {
  const delta = (b.strength?.score ?? 0) - (a.strength?.score ?? 0);
  return delta !== 0 ? delta : stable(a.person_id, b.person_id);
}

type Organization360Contact = NonNullable<
  Organization360["people"]
>["data"][number];
