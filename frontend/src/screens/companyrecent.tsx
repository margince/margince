// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What happened lately, as a list to scan rather than a chronicle to read.
//
// The record's own History tab is the chronicle: day headings, message bodies,
// signatures, provenance on every row. Inside the account's reading this
// section answers a narrower question — has anything moved, and which way —
// and the chronicle answered it in five screens. One row per exchange, stating
// what KIND it was, which direction it went, what it was about and when.
//
// Its own renderer rather than a mode of the timeline: the two disagree about
// what a row IS, not about how one looks, and a `compact` flag threaded
// through the timeline would be that disagreement spelled as a boolean.

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Avatar, Badge } from "../design-system/atoms";
import { PanelBody } from "../design-system/panel";
import { formatDateAbbrev, formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import "./company360.css";

type Activity = components["schemas"]["Activity"];
type ActivityKind = Activity["kind"];

const KIND_LABELS: Record<ActivityKind, MessageKey> = {
  email: "co.recent.kind.email",
  call: "co.recent.kind.call",
  meeting: "co.recent.kind.meeting",
  note: "co.recent.kind.note",
  task: "co.recent.kind.task",
  message: "co.recent.kind.message",
};

/**
 * Which way the exchange went, in the words a rep would use.
 *
 * "They wrote" and "we sent" rather than "inbound" and "outbound": the row is
 * read to decide whether anybody owes anybody a reply, and that is a question
 * about people. A meeting has both sides by definition and says so; a note has
 * no direction at all and says nothing rather than inventing one.
 */
function directionLabel(activity: Activity): MessageKey | undefined {
  if (activity.kind === "meeting") {
    return "co.recent.dir.both";
  }
  if (activity.direction === "inbound") {
    return activity.kind === "call"
      ? "co.recent.dir.theyCalled"
      : "co.recent.dir.theyWrote";
  }
  if (activity.direction === "outbound") {
    return activity.kind === "call"
      ? "co.recent.dir.weCalled"
      : "co.recent.dir.weSent";
  }
  return undefined;
}

/** How long a meeting or a call ran, when the row carries it. */
function durationLabel(
  activity: Activity,
  t: ReturnType<typeof useT>,
  locale: Locale,
) {
  const seconds = activity.duration_seconds;
  if (seconds == null || seconds <= 0) {
    return undefined;
  }
  return t("co.recent.minutes", {
    count: formatNumber(Math.round(seconds / 60), locale),
  });
}

/**
 * CompanyRecentList is the exchanges, newest first.
 *
 * The mark carries the row rather than an icon per kind: a reader scanning the
 * list is looking for a person they know, and the KIND already has a word of
 * its own beside it. Withheld rows still draw — a section that silently
 * dropped what this reader may not see would report a quieter account than the
 * one on file.
 */
export function CompanyRecentList({
  activities,
  onOpenRecord,
}: Readonly<{
  activities: readonly Activity[];
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <PanelBody className="co-recent">
      <ul className="co-recent-list">
        {activities.map((activity) => (
          <RecentRow
            key={activity.id}
            activity={activity}
            when={formatDateAbbrev(activity.occurred_at, locale, zone)}
            direction={directionLabel(activity)}
            duration={durationLabel(activity, t, locale)}
            onOpenRecord={onOpenRecord}
          />
        ))}
      </ul>
    </PanelBody>
  );
}

function RecentRow({
  activity,
  when,
  direction,
  duration,
  onOpenRecord,
}: Readonly<{
  activity: Activity;
  when: string;
  direction?: MessageKey;
  duration?: string;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const kind = t(KIND_LABELS[activity.kind]);
  // The subject when there is one, the kind's own word when there is not. A
  // call logged with no subject is still an event, and a blank line where the
  // headline goes reads as a row that failed to load.
  const title = activity.subject?.trim() || kind;
  const deal = activity.links?.find((link) => link.entity_type === "deal");
  return (
    <li className="co-recent-row">
      <Avatar name={title} identity={activity.id} size="xs" />
      <span className="co-recent-body">
        <span className="co-recent-head">
          <Badge tone="ai">{kind}</Badge>
          {direction && <span className="co-recent-dir">{t(direction)}</span>}
        </span>
        <span className="co-recent-title">{title}</span>
        <span className="co-recent-meta">
          {duration && <span>{duration}</span>}
          {deal &&
            (onOpenRecord ? (
              <button
                type="button"
                className="co-rowlink"
                onClick={() => onOpenRecord("deal", deal.entity_id)}
              >
                {t("co.recent.re")}
              </button>
            ) : (
              <span>{t("co.recent.re")}</span>
            ))}
        </span>
      </span>
      <span className="co-recent-when t-mono">{when}</span>
    </li>
  );
}
