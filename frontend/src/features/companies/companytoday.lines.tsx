import type { ReactNode } from "react";
import { EntityRef } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { formatDateTime, formatNumber } from "../../format/format";
import type { Locale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { TodoRow, WrittenBy } from "../records/record360";
import { type AccountScan, scanHasSettled, scanIsLive } from "./accountscan";
import type { nextCommitmentLine } from "./company360";
import type { Company360 } from "./companytoday";
import { byStrengthThenId } from "./companytodayverbs";
import "./company360.css";

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
    const resumeKey =
      scan.degrade_reason === "provider_deferred"
        ? "today.scan.providerResumes"
        : scan.degrade_reason === "budget_deferred"
          ? "today.scan.resumes"
          : "today.scan.scheduledResumes";
    return (
      <p className="co-row-meta">
        {t(resumeKey, {
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
        // Not indigo: the composer opens empty and waits for the rep to say
        // what the message is for, so the press itself starts no model.
        verb={{
          label: t("today.draft.act"),
          onAct: () => onDraftTo(recipient.contact_id),
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
