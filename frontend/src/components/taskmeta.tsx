// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The chip line under a piece of work: when it is due, what kind it is, who and
// what it is with, and its state. Home's cards and the Worklist's rows share it.

import {
  CalendarClock,
  CalendarDays,
  Hourglass,
  type LucideIcon,
  Mail,
  MessageSquare,
  Settings,
  ShieldCheck,
  SquareCheck,
  TrendingDown,
  UserPlus,
} from "lucide-react";
import { Badge } from "../design-system/atoms";
import { Chip } from "../design-system/readings";
import { activityDay, activityTimeOfDay } from "../format/activitywhen";
import { formatDayMonthNearYear } from "../format/format";
import { useLocale, usePlural, useT, type Words } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { changeBadge } from "../lib/calendarchange";
import { useCompanyZone } from "../lib/companyzone";
import { valueFor } from "../lib/labelfor";
import { isTaskType, type TaskType } from "../lib/tasktype";
import { useViewerZone } from "../lib/viewerzone";
import {
  briefSubjectLabel,
  contactHref,
  employerOf,
  subjectHref,
  whenText,
} from "../lib/worklistcopy";
import { emailNamesRow } from "../lib/worklistemailrow";
import { conditionOf, eyebrowKeyFor } from "../lib/worklisteyebrow";
import type { WorklistItem } from "../lib/worklisttypes";
import { RECORD_CHIP_ICONS, RecordMarkChip } from "./recordlinks";
import { TaskTypeChip } from "./tasktype";
import "./taskmeta.css";

/**
 * One line of chips under a card's text. It says when the work is due, what
 * kind it is, and who and what it is with; its state sits at the far end.
 * `link` makes each record a link on the open card, which is no button.
 */
export function TaskMeta({
  item,
  link,
  taskType,
}: Readonly<{
  item: WorklistItem;
  link?: (href: string) => string;
  /** The task's type from its own read, over the one the row carries. */
  taskType?: TaskType;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  // The due day is the server's bucket, counted on the company's clock.
  const companyZone = useCompanyZone();
  const when = cardWhen(
    item,
    { t, plural, locale },
    readerZone,
    companyZone,
    new Date(),
  );
  // The open card's title already carries the late badge.
  const status = statusOf(item, link !== undefined);
  const type = taskType ?? sentType(item);
  return (
    <span className="task-meta">
      {when && (
        <Badge tone={dueTone(item)} icon={CalendarDays}>
          {when}
        </Badge>
      )}
      {type ? (
        <TaskTypeChip type={type} />
      ) : (
        <Chip dense icon={kindIcon(item)}>
          {conditionOf(item, t) ?? t(kindKey(item))}
        </Chip>
      )}
      <TaskRecords item={item} link={link} />
      {status && (
        <span className="task-meta-status">
          <Badge tone={status.tone} dot>
            {t(status.key)}
          </Badge>
        </span>
      )}
    </span>
  );
}

// The row's own type, where it carries one this build knows.
function sentType(item: WorklistItem): TaskType | undefined {
  const sent = item.task_type;
  return sent && isTaskType(sent) ? sent : undefined;
}

const CATEGORY_ICONS: Record<WorklistItem["category"], LucideIcon> = {
  customer_waiting: MessageSquare,
  leads: UserPlus,
  deals_at_risk: TrendingDown,
  meetings: CalendarClock,
  upcoming: CalendarClock,
  tasks: SquareCheck,
  decisions: ShieldCheck,
  relationships: Hourglass,
  system: Settings,
};

// A waiting message is named for what it is, an email; its category moves to
// the state at the end of the line. A promise carrying its email stays a task.
function kindKey(item: WorklistItem): MessageKey {
  if (item.email_summary?.calendar_change) return "timeline.kind.meeting";
  return emailNamesRow(item) ? "timeline.kind.email" : eyebrowKeyFor(item);
}

function kindIcon(item: WorklistItem): LucideIcon {
  if (item.email_summary?.calendar_change) return CalendarClock;
  return emailNamesRow(item)
    ? Mail
    : valueFor(CATEGORY_ICONS, item.category, SquareCheck);
}

type CardStatus = Readonly<{
  key: MessageKey;
  tone: "danger" | "warning" | "info" | "success";
}>;

/** Late first, then why a message is waiting; any other row has no state. */
function statusOf(item: WorklistItem, titled: boolean): CardStatus | null {
  if (item.overdue === true)
    return titled ? null : { key: "worklist.overdue", tone: "danger" };
  const change = item.email_summary?.calendar_change;
  if (change) return changeBadge(change);
  if (item.email_summary?.calendar_invitation)
    return { key: eyebrowKeyFor(item), tone: "info" };
  if (emailNamesRow(item) && item.band === "now")
    return { key: eyebrowKeyFor(item), tone: "warning" };
  return null;
}

/**
 * The clock a card races. An undated task has none, so it shows none. A late
 * one says its day and how late it is, as whenText writes it for the Worklist.
 */
export function cardWhen(
  item: WorklistItem,
  words: Words,
  zone: string,
  recordZone: string,
  now: Date,
): string | null {
  if (item.source === "task" && !item.due_at) return null;
  const { t, locale } = words;
  // A waiting message is dated by its arrival, as the row in the queue is.
  const arrived = emailNamesRow(item) ? item.email_summary : null;
  if (arrived) {
    const { occurred_at: at, occurred_precision: precision } = arrived;
    return [
      activityDay(at, precision, locale, zone, (day, lang, read) =>
        formatDayMonthNearYear(day, lang, read, now),
      ),
      activityTimeOfDay(at, precision, locale, zone),
    ]
      .filter(Boolean)
      .join(", ");
  }
  return whenText(item, t, locale, zone, recordZone, now);
}

/** Late is danger, the day's first band a warning, anything else plain. */
export function dueTone(item: WorklistItem): "danger" | "warning" | "default" {
  if (item.overdue === true) return "danger";
  return item.band === "now" ? "warning" : "default";
}

type Named = Readonly<{
  id: string;
  label: string;
  href?: string;
  mark: "avatar" | LucideIcon;
}>;

// The task details panel draws its records with the same marks.
const SUBJECT_ICONS: Partial<Record<string, LucideIcon>> = RECORD_CHIP_ICONS;

/**
 * Who the work is with and what it is about. That is the contact, then their
 * account, then the row's record where it is neither.
 */
function TaskRecords({
  item,
  link,
}: Readonly<{ item: WorklistItem; link?: (href: string) => string }>) {
  // A brief item's card is named by its record (taskName), so no chip repeats it.
  const headed = briefSubjectLabel(item) !== undefined;
  const named = namedOn(item).filter(
    (who, index, all) =>
      all.findIndex((other) => other.id === who.id) === index &&
      !(headed && who.id === item.subject?.id),
  );
  return named.map((who) => {
    const appHref = link && who.href ? link(who.href) : undefined;
    return (
      <RecordMarkChip
        key={who.id}
        id={who.id}
        name={who.label}
        icon={who.mark === "avatar" ? undefined : who.mark}
        href={appHref}
      />
    );
  });
}

/** The subject has a chip of its own on the card, so the name need not say it. */
export function subjectChipped(item: WorklistItem): boolean {
  const subject = item.subject;
  if (!subject?.label) return false;
  return (
    subject.type === "contact" ||
    subject.type === "company" ||
    SUBJECT_ICONS[subject.type] !== undefined
  );
}

function namedOn(item: WorklistItem): Named[] {
  const named: Named[] = [];
  const contact = item.contact;
  if (contact?.label)
    named.push({
      id: contact.id,
      label: contact.label,
      href: contactHref(contact),
      mark: "avatar",
    });
  const employer = contact && employerOf(contact);
  if (contact?.employer && employer)
    named.push({
      id: contact.employer.company_id,
      label: employer.label,
      href: employer.href,
      mark: "avatar",
    });
  const subject = item.subject;
  if (subject?.label) {
    const mark =
      subject.type === "contact" || subject.type === "company"
        ? "avatar"
        : SUBJECT_ICONS[subject.type];
    if (mark)
      named.push({
        id: subject.id,
        label: subject.label,
        href: subjectHref(item),
        mark,
      });
  }
  return named;
}
