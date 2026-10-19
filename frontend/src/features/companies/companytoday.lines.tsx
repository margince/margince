import type { ReactNode } from "react";
import { EntityRef } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { formatDateTime, formatNumber } from "../../format/format";
import type { Locale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { TodoRow, WrittenBy } from "../../screens/record360";
import {
  type AccountScan,
  scanHasSettled,
  scanIsLive,
} from "../capture/accountscan";
import {
  ENGAGEMENT_LABELS,
  ENGAGEMENT_TONE,
  type nextCommitmentLine,
} from "./company360";
import type { Company360 } from "./companytoday";
import { byStrengthThenId } from "./companytodayverbs";
import "./company360.css";

// The lead reading: whose move it is, under the call and in its own weight. It
// is the one thing a reader must know before the moves under it mean anything.
type TodayLead = {
  headline: string;
  // How long it has stood that way, beside the state and quieter than it. A
  // state with no duration is a status; with one it is a fact a rep can act
  // on.
  note?: string;
  // Colours the state where it is bad news: an account gone quiet, a move
  // that has been ours for weeks.
  tone?: "warning" | "danger";
};

// The band under the rows: what the brief as a whole is counting down to, and
// what the advice could not show. Undefined when there is neither: Panel
// draws the band on truthiness, and an empty band wastes a row.
//
// The way out of the brief is not here. A footer holding one link is that
// same wasted row, so "View tasks" sits beside the panel's name.
export function briefFooter(
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
        <Badge tone={commitment.overdue ? "warning" : undefined}>
          {commitment.headline}
        </Badge>
      )}
      {fromAdvice}
      {fromScan}
    </>
  );
}

/**
 * The read's own line under the rows: who wrote the findings and what was read.
 * Beside the rows, it also says that the account has moved since, or that the
 * AI budget put the read off. Nothing until a read has settled, because a
 * running scan has no reading to describe yet.
 */
export function scanFoot({
  scan,
  plural,
  t,
  locale,
  recordZone,
}: Readonly<{
  scan?: AccountScan;
  t: ReturnType<typeof useT>;
  plural: ReturnType<typeof usePlural>;
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
            exchanges: plural("today.scan.readExchanges", scan.read.exchanges, {
              count: formatNumber(scan.read.exchanges, locale),
            }),
            deals: plural("today.scan.readDeals", scan.read.deals, {
              count: formatNumber(scan.read.deals, locale),
            }),
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

export function leadSentence(lead: TodayLead): ReactNode {
  return (
    <>
      <span>{lead.headline}</span>
      {lead.note && <span className="today-lead-note t-sub">{lead.note}</span>}
    </>
  );
}

export function manualMoveRows({
  view,
  t,
  onPrepareMeeting,
  onDraftTo,
  hasDraftReply,
}: Readonly<{
  view: Company360;
  t: ReturnType<typeof useT>;
  onPrepareMeeting?: (activityId: string) => void;
  onDraftTo?: (contactId: string) => void;
  // Whether the advice above already offers a reply to a specific message.
  hasDraftReply: boolean;
}>): ReactNode[] {
  const recipient = [...(view.contacts?.data ?? [])].sort(byStrengthThenId)[0];
  const meeting = view.next_meeting;
  const rows: ReactNode[] = [];
  // The generic row is dropped when a suggestion already says "answer this
  // message". The generic one picks the account's strongest contact, often not
  // the one waiting on a reply. Two draft rows naming two contacts would tell a
  // rep two different things.
  if (recipient && onDraftTo && !hasDraftReply) {
    rows.push(
      <TodoRow
        key="move:draft"
        // `who` is plain data for the avatar; the linked name goes in `meta`,
        // which keeps English word order out of the label.
        who={{ name: recipient.full_name, identity: recipient.contact_id }}
        title={t("today.draft.new")}
        meta={
          <EntityRef
            kind="contact"
            id={recipient.contact_id}
            name={recipient.full_name}
          />
        }
        verb={{
          label: t("today.draft.act"),
          onAct: () => onDraftTo(recipient.contact_id),
          byMargince: true,
        }}
      />,
    );
  }
  if (meeting && onPrepareMeeting) {
    const who =
      meeting.participants.length > 0
        ? meeting.participants.map((participant, at) => (
            <span key={participant.contact_id}>
              {at > 0 && ", "}
              <EntityRef
                kind="contact"
                id={participant.contact_id}
                name={participant.display_name}
              />
            </span>
          ))
        : undefined;
    rows.push(
      <TodoRow
        key="move:meeting"
        who={{
          name: meeting.participants[0]?.display_name ?? meeting.subject,
          identity: meeting.participants[0]?.contact_id ?? meeting.activity_id,
        }}
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

export function omitted(view: Company360, section: string): boolean {
  return (view.sections_omitted ?? []).some((name) => name === section);
}

export const TODAY_SOURCES: ReadonlyArray<{
  section: string;
  label: MessageKey;
}> = [
  { section: "next_steps", label: "today.source.nextSteps" },
  { section: "next_meeting", label: "today.source.nextMeeting" },
  { section: "contacts", label: "today.source.contacts" },
  { section: "deals", label: "today.source.deals" },
  { section: "state_strip", label: "today.source.standing" },
  { section: "activities", label: "today.source.activities" },
  { section: "suggestions", label: "today.source.suggestions" },
];

export function whoseMove({
  view,
  t,
  locale,
}: Readonly<{
  view: Company360;
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
  view: Company360,
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
