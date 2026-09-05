// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { AiPending } from "../design-system/aipending";
import { Badge, Button, EmptyState, Skeleton } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { PanelBody, PanelRow } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import { stable } from "../format/collate";
import { formatDateTime, formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type AccountScan, scanHasSettled, scanIsLive } from "./accountscan";
import {
  ENGAGEMENT_LABELS,
  ENGAGEMENT_TONE,
  nextCommitmentLine,
  type SuggestionAction,
  useSuggestionsBody,
} from "./company360";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_DIMENSION_MEANS,
  HEALTH_RATING_LABEL,
  HEALTH_STANDING_TONE,
  useAccountStanding,
} from "./companylookups";
import { EntityRef } from "./entityref";
import {
  MOMENT_RULE_LABEL,
  momentGrounding,
  standingTone,
} from "./persontoday";
import {
  CallCard,
  type Grounding,
  Proof,
  type StandingTone,
  TodayPanel,
  TodoRow,
  WithheldNotice,
  WrittenBy,
} from "./record360";
import "./company360.css";

type Organization360 = components["schemas"]["Organization360"];
type HealthRating = components["schemas"]["HealthDimension"]["rating"];
type PersonMoment = components["schemas"]["PersonMoment"];

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

/** One of the three rated dimensions under the verdict. */
export type TodayDimension = {
  key: "relationship" | "commercial" | "payment";
  // What is rated.
  label: string;
  // The rating in words — or, where there is none, why: nothing read yet, or
  // nothing shown to this reader. Never absent, because the chip states a
  // dimension's standing and a blank one would read as a rating of zero.
  reading: string;
  // How loud the rating is. Absent where there is no rating to be loud about.
  tone?: "calm" | "warn" | "danger";
  // What this dimension WEIGHS. Three words on a card cannot say what went
  // into "Commercial · Good", and a reader who cannot interpret a rating has
  // to take it on trust.
  means: string;
  // The reading the rating was made from, in the rule's own words. Absent
  // where nothing was rated — there is then no working to show.
  because?: string;
};

/**
 * The account's reading for today, computed ONCE and drawn in two panes: the
 * 360's call (the word, the sentence it rests on, the dimensions) and the
 * needs list (the moment, the agent's finds, the manual moves). Two panes
 * because the glance puts them in different cells of the page; one reading
 * because the verdict and the queue under it come off the same hooks, and two
 * computations of "how is this account" would agree until one learned about
 * payment and the other did not.
 */
export type TodayReading =
  | { state: "loading" }
  | { state: "failed" }
  | {
      state: "ready";
      standing: { label: string; tone: StandingTone };
      because?: ReactNode;
      restsOn: readonly Grounding[];
      dimensions: readonly TodayDimension[];
      rows: ReactNode[];
      footer?: ReactNode;
      notice?: ReactNode;
    };

// What a reading is computed from: the account's composite read and the
// verbs the surface above can perform on what it says.
type TodayReadingInputs = Readonly<{
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
  // The reader's scan of the account, when the page holds one: its merged
  // advice replaces the 360's own rows, a read in flight draws the pending
  // row, and the foot says who read what and when.
  scan?: AccountScan;
}>;

export function useTodayReading({
  orgId,
  view,
  loading,
  failed,
  onPrepareMeeting,
  onDraftTo,
  onOpenRecord,
  onPerform,
  scan,
}: TodayReadingInputs): TodayReading {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Called before the loading/failed branches below, like every other hook
  // here: React requires it, and it answers "nothing rated" on its own.
  const verdict = useAccountStanding(orgId, view?.health);
  const suggestions = useSuggestionsBody({
    orgId,
    view,
    onOpenRecord,
    onPerform,
    advice: scan
      ? { findings: scan.findings, dropped: scan.findings_dropped }
      : undefined,
  });
  if (loading) {
    return { state: "loading" };
  }
  if (failed || !view) {
    return { state: "failed" };
  }
  const lead = whoseMove({ view, t, locale });
  const commitment = nextCommitmentLine(view, locale, t);
  // Nothing rated is itself a standing, and it says so: an account too new to
  // score reads "not assessed" rather than dropping the call and leaving the
  // sentence under it with nothing to belong to. The words are the readings
  // row's own for the same state, so the two cannot disagree.
  // A withheld health section is a fact about the READER and says so;
  // nothing rated is a fact about how much has been judged. The two must not
  // share a word, or a reader goes asking for a grant that would show them
  // nothing.
  const healthWithheld = omitted(view, "health");
  const standing = verdict.overall
    ? {
        label: t(HEALTH_RATING_LABEL[verdict.overall]),
        tone: HEALTH_STANDING_TONE[verdict.overall],
      }
    : {
        label: t(healthWithheld ? "record.notShown" : "co.strip.notAssessed"),
        tone: "unknown" as const,
      };
  const rated: ReadonlyArray<
    [TodayDimension["key"], HealthRating | undefined]
  > = [
    ["relationship", view.health?.relationship?.rating],
    ["commercial", view.health?.commercial?.rating],
    ["payment", verdict.payment?.rating],
  ];
  const dimensions: TodayDimension[] = rated.map(([key, rating]) => ({
    key,
    label: t(HEALTH_DIMENSION_LABEL[key]),
    reading: rating
      ? t(HEALTH_RATING_LABEL[rating])
      : t(healthWithheld ? "record.notShown" : "co.strip.notAssessed"),
    tone: rating ? HEALTH_STANDING_TONE[rating] : undefined,
    means: t(HEALTH_DIMENSION_MEANS[key]),
    // The working, from the same place the call's own grounding is built, so
    // a dimension's chip and the verdict's "what this rests on" cannot come
    // to quote two different readings of one dimension.
    because: verdict.restsOn.find((reading) => reading.key === key)?.quote,
  }));
  // WHAT WE OWE leads the list. A promise past its date outranks a reading of
  // the account: one is a thing to do today and the other is context for it.
  const rows: ReactNode[] = [
    ...(view.moment
      ? [
          <MomentRow
            key="moment"
            moment={view.moment}
            onOpenRecord={onOpenRecord}
          />,
        ]
      : []),
    // The read in flight, above the rows it will add to: the rules' rows
    // stand while Margince reads, and the pending row is what says more is
    // coming rather than that this is everything.
    ...(scanIsLive(scan)
      ? [
          <PanelRow key="scan" className="co-move co-move-reading">
            <AiPending
              label={t(
                scan?.state === "running"
                  ? "today.scan.reading"
                  : "today.scan.queued",
              )}
              lines={2}
            />
          </PanelRow>,
        ]
      : []),
    suggestions.rows,
    ...manualMoveRows({ view, t, onPrepareMeeting, onDraftTo }),
  ];
  return {
    state: "ready",
    standing,
    because: lead ? leadSentence(lead) : undefined,
    restsOn: verdict.restsOn,
    dimensions,
    rows,
    footer: briefFooter(
      commitment,
      suggestions.footer,
      scanFoot({ scan, t, locale, recordZone }),
    ),
    notice:
      (view.sections_omitted?.length ?? 0) > 0 ? (
        <TodayWithheld view={view} />
      ) : undefined,
  };
}

/**
 * One rated dimension, and what stands behind it.
 *
 * The chip is a TRIGGER, because three words cannot carry a rating a reader
 * can check: "Commercial · Good" says neither what was weighed nor what it was
 * read from, so a reader either takes it on trust or goes looking for the
 * working somewhere else on the page. Resting on the chip answers both, in
 * that order — what the dimension is, then the reading the rating was made
 * from.
 *
 * `onHover` and not a click alone: this is an aside taken in on the way past,
 * where a click is a step a reader should not have to take to check a claim.
 * The click stays for the touch screen and the keyboard, which have no hover.
 */
function DimensionChip({ dimension }: Readonly<{ dimension: TodayDimension }>) {
  const t = useT();
  return (
    <Popover
      onHover
      className={dimension.tone ? `co-dim co-dim-${dimension.tone}` : "co-dim"}
      label={`${dimension.label} · ${dimension.reading}`}
    >
      <p className="co-dim-means">{dimension.means}</p>
      {dimension.because ? (
        <>
          {/* The same words the verdict's own grounding uses, over the same
              quote: two names for one working would read as two readings. A
              label beside a value, not a heading: the panel is already named
              by the chip that opened it. */}
          <Eyebrow className="co-dim-restson">{t("record.restsOn")}</Eyebrow>
          <p className="co-dim-quote">{dimension.because}</p>
        </>
      ) : null}
    </Popover>
  );
}

/**
 * The 360's call: the head whose mark says a machine read this record, the
 * standing word with the sentence it rests on, the three rated dimensions, and
 * under them whatever the call was read from — the spine, the folded thread.
 *
 * While the read is in flight the card holds its shape and no verdict: a head
 * holding a spinner where the call goes is the reading claiming to have
 * reached one.
 */
export function Company360Call({
  reading,
  name,
  footer,
  children,
}: Readonly<{
  reading: TodayReading;
  name?: string;
  footer?: ReactNode;
  children?: ReactNode;
}>) {
  const t = useT();
  if (reading.state === "loading") {
    return (
      <CallCard name={name}>
        <PanelBody>
          <Skeleton width="100%" height={64} />
        </PanelBody>
      </CallCard>
    );
  }
  if (reading.state === "failed") {
    return (
      <CallCard name={name}>
        <PanelBody>
          <EmptyState>{t("co.section.unavailable")}</EmptyState>
        </PanelBody>
      </CallCard>
    );
  }
  return (
    <CallCard
      name={name}
      standing={reading.standing}
      because={reading.because}
      restsOn={reading.restsOn}
      footer={footer}
    >
      <PanelBody className="co-360-dims">
        {reading.dimensions.map((dimension) => (
          <DimensionChip key={dimension.key} dimension={dimension} />
        ))}
      </PanelBody>
      {children}
    </CallCard>
  );
}

/**
 * What needs a person: one list, the moment as its lead row, then the agent's
 * finds, then the manual moves. The foot carries what the list as a whole is
 * counting down to and what the advice could not show.
 */
export function NeedsList({
  reading,
  onOpenTasks,
}: Readonly<{
  reading: TodayReading;
  // Where the footer's commitment reading leads. Absent for a caller with no
  // Tasks tab of its own.
  onOpenTasks?: () => void;
}>) {
  if (reading.state !== "ready") {
    return <TodayPanel state={reading.state} onOpenTasks={onOpenTasks} />;
  }
  return (
    <TodayPanel
      onOpenTasks={onOpenTasks}
      footer={reading.footer}
      notice={reading.notice}
    >
      {reading.rows}
    </TodayPanel>
  );
}

/**
 * The call and the needs list as one column, for a surface with no glance
 * grid of its own: the two panes in the order the page draws them.
 */
export function TodayOnThisAccount({
  onOpenTasks,
  spine,
  ...inputs
}: TodayReadingInputs &
  Readonly<{
    onOpenTasks?: () => void;
    // The account's story as a thread, under the call it was read from.
    spine?: ReactNode;
  }>) {
  const reading = useTodayReading(inputs);
  return (
    <>
      <Company360Call
        reading={reading}
        name={inputs.view?.organization?.display_name}
      >
        {spine}
      </Company360Call>
      <NeedsList reading={reading} onOpenTasks={onOpenTasks} />
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
  fromScan: ReactNode,
): ReactNode | undefined {
  if (!commitment && !fromAdvice && !fromScan) {
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
      {fromScan}
    </>
  );
}

/**
 * The read's own line under the rows: who wrote the findings, what was read,
 * and — said beside the rows rather than instead of them — that the account
 * has moved since, or that the AI budget put the read off. Nothing until a
 * read has settled: a foot under a scan still running would be describing a
 * reading that does not exist yet.
 */
function scanFoot({
  scan,
  t,
  locale,
  recordZone,
}: Readonly<{
  scan?: AccountScan;
  t: ReturnType<typeof useT>;
  locale: Locale;
  recordZone: string;
}>): ReactNode {
  if (!scan) {
    return null;
  }
  if (scanIsLive(scan) && scan.resumes_at) {
    return (
      <p className="co-row-meta">
        {t("today.scan.resumes", {
          when: formatDateTime(scan.resumes_at, locale, recordZone),
        })}
      </p>
    );
  }
  if (!scanHasSettled(scan) || !scan.generated_by) {
    return null;
  }
  return (
    <span className="co-scan-foot">
      <WrittenBy by={scan.generated_by} />
      {scan.read && (
        <span className="co-row-meta">
          {t("today.scan.read", {
            exchanges: formatNumber(scan.read.exchanges, locale),
            deals: formatNumber(scan.read.deals, locale),
          })}
        </span>
      )}
      {scan.stale && (
        <span className="co-row-meta">{t("today.scan.stale")}</span>
      )}
      {/* Server-authored, in the reader's terms: why the model did not
          write these rows. Shown as-is, like a rule's own reason. */}
      {scan.degrade_reason && (
        <span className="co-row-meta">{scan.degrade_reason}</span>
      )}
    </span>
  );
}

function leadSentence(lead: TodayLead): ReactNode {
  return (
    <>
      <span className="today-lead-state">{lead.headline}</span>
      {lead.note && <span className="today-lead-note t-sub">{lead.note}</span>}
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
  if (recipient && onDraftTo) {
    rows.push(
      <TodoRow
        key="move:draft"
        who={recipient.full_name}
        title={t("today.draft.to", { name: firstName(recipient.full_name) })}
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

function omitted(view: Organization360, section: string): boolean {
  return (view.sections_omitted ?? []).some((name) => name === section);
}

const TODAY_SOURCES: ReadonlyArray<{ section: string; label: MessageKey }> = [
  { section: "next_steps", label: "today.source.nextSteps" },
  { section: "next_meeting", label: "today.source.nextMeeting" },
  { section: "people", label: "today.source.people" },
  { section: "deals", label: "today.source.deals" },
  { section: "state_strip", label: "today.source.standing" },
  { section: "activities", label: "today.source.activities" },
  { section: "suggestions", label: "today.source.suggestions" },
];

function TodayWithheld({ view }: Readonly<{ view: Organization360 }>) {
  const t = useT();
  const hidden = TODAY_SOURCES.filter((source) =>
    omitted(view, source.section),
  );
  return <WithheldNotice sections={hidden.map((source) => t(source.label))} />;
}

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
  return days > 0
    ? t("today.silence.days", { count: formatNumber(days, locale) })
    : undefined;
}

const MS_PER_DAY = 24 * 60 * 60 * 1000;

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

/**
 * The moment as the lead row of the needs list: the rule it fired on as the
 * eyebrow, the headline in the display face, why now, the evidence one
 * disclosure away, and the one filled verb on the page.
 *
 * The button appears only where the server said the action can be taken AND
 * named somewhere to go. A card whose verb lands nowhere is worse than a card
 * with no verb: the reader clicks, nothing happens, and they stop trusting
 * the ones that work.
 */
function MomentRow({
  moment,
  onOpenRecord,
}: Readonly<{
  moment: PersonMoment;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const destination = moment.recommended_action.destination;
  const target =
    moment.recommended_action.state === "available" &&
    destination?.entity_type != null &&
    destination.entity_id != null
      ? { type: destination.entity_type, id: destination.entity_id }
      : undefined;
  const tone = standingTone(moment.rule);
  return (
    <PanelRow className="co-move co-move-lead">
      <span className="co-move-body">
        <span className="co-move-by">
          <span className={`co-dim co-dim-${tone}`}>
            {t(MOMENT_RULE_LABEL[moment.rule])}
          </span>
        </span>
        <span className="co-move-ask co-move-headline">{moment.headline}</span>
        <span className="co-move-reason t-sub">{moment.why_now}</span>
        <Proof
          label={t("record.restsOn")}
          items={momentGrounding(moment.evidence, t, locale, recordZone)}
          count
        />
        {target && onOpenRecord && (
          <span className="co-move-do">
            <span className="co-move-actions">
              {/* Indigo, because pressing it hands the work to Margince: the
                  hue is the product's one claim about who is acting, and a
                  verb the agent performs drawn in the accent would read as
                  the reader's own move. */}
              <Button
                small
                variant="ai"
                onClick={() => onOpenRecord(target.type, target.id)}
              >
                {moment.recommended_action.label}
              </Button>
            </span>
          </span>
        )}
      </span>
    </PanelRow>
  );
}
