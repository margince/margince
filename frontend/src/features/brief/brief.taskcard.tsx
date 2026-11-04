// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One row of the day as a to-do card: the tick, what to do, and who it is
// with. The row in hand is drawn whole by brief.focus.tsx; these are the rest.

import {
  CalendarClock,
  CalendarDays,
  Circle,
  FolderKanban,
  Handshake,
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
import { useTaskDone } from "../../components/taskactions";
import { Badge, Card } from "../../design-system/atoms";
import { Avatar } from "../../design-system/avatar";
import { IconAction } from "../../design-system/iconaction";
import { Chip } from "../../design-system/readings";
import {
  calendarDaysBetween,
  formatDayMonthNearYear,
  formatNumber,
} from "../../format/format";
import { viewerZone } from "../../format/timezone";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
  type Words,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";
import { useMe } from "../../lib/session";
import {
  briefContextTitle,
  briefSubjectLabel,
  contactHref,
  employerOf,
  itemTitle,
  phrasedReasons,
  readerTask,
  reasonText,
  subjectHref,
  whenText,
} from "../../lib/worklistcopy";
import { eyebrowKeyFor } from "../../lib/worklisteyebrow";
import type { WorklistItem } from "../../lib/worklisttypes";

/** The name a card goes by, the way the row in hand names it. */
export function taskName(
  item: WorklistItem,
  viewer: Parameters<typeof readerTask>[1],
  t: Translator,
  locale: Locale,
): string {
  const mail = item.email_summary;
  const sender = item.contact?.label ?? mail?.counterparty;
  // A message waiting on the reader is named by the move it asks for.
  if (mail?.move === "needs_reply" && sender && mail.subject)
    return t("brief.task.reply", { name: sender, subject: mail.subject });
  return (
    briefSubjectLabel(item) ??
    itemTitle(readerTask(item, viewer, t), t, locale, subjectChipped(item))
  );
}

/**
 * The line under a card's name: what to do. The agent's own move first, then
 * the first reason the row is here, then the producer's own supporting line.
 * Null when the row has none of them: the kind's stock sentence on every card
 * would be one line said six times.
 */
export function taskLine(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
): string | null {
  // The reader's own pin is the queue's reason, not the day's.
  const reason = phrasedReasons(item, false)
    .filter((entry) => entry.kind !== "pinned")
    .map((entry) => reasonText(entry, t, locale, zone))
    .find((text) => text !== null);
  return briefContextTitle(item) ?? reason ?? (item.detail || null);
}

/**
 * The tick that marks a task done. Only a task the reader may complete has
 * one; any other row keeps the column empty, so every name starts on one x.
 */
export function TaskTick({ item }: Readonly<{ item: WorklistItem }>) {
  const completes =
    item.source === "task" && !item.batch && item.actions.includes("complete");
  return (
    <span className="brief-task-tick">
      {completes && <DoneVerb item={item} />}
    </span>
  );
}

function DoneVerb({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const viewer = useMe(false).data?.user;
  const done = useTaskDone(item.id, item.version);
  return (
    <IconAction
      label={t("brief.task.done", {
        title: taskName(item, viewer, t, locale),
      })}
      icon={<Circle aria-hidden="true" />}
      variant="ghost"
      pending={done.pending}
      onClick={() => void done.complete()}
    />
  );
}

/** A row still waiting behind the one in hand; a press puts it in hand. */
export function TaskCard({
  item,
  onOpen,
}: Readonly<{ item: WorklistItem; onOpen: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const viewer = useMe(false).data?.user;
  const line = taskLine(item, t, locale, zone);
  return (
    <Card as="article" className="brief-task">
      <TaskTick item={item} />
      <button type="button" className="brief-task-open" onClick={onOpen}>
        <span className="brief-task-title">
          {taskName(item, viewer, t, locale)}
        </span>
        {line && <span className="brief-task-line">{line}</span>}
        <TaskMeta item={item} />
      </button>
    </Card>
  );
}

/**
 * One line of chips under a card's text. It says when the work is due, what
 * kind it is, and who and what it is with; its state sits at the far end.
 * `link` makes each record a link on the open card, which is no button.
 */
export function TaskMeta({
  item,
  link,
}: Readonly<{ item: WorklistItem; link?: (href: string) => string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const when = cardWhen(
    item,
    { t, plural, locale },
    viewerZone(),
    recordZone,
    new Date(),
  );
  // The open card's title already carries the late badge.
  const status = statusOf(item, link !== undefined);
  return (
    <span className="brief-task-meta">
      {when && (
        <Badge tone={dueTone(item)} icon={CalendarDays}>
          {when}
        </Badge>
      )}
      <Chip dense icon={kindIcon(item)}>
        {t(kindKey(item))}
      </Chip>
      <TaskRecords item={item} link={link} />
      {status && (
        <span className="brief-task-status">
          <Badge tone={status.tone} dot>
            {t(status.key)}
          </Badge>
        </span>
      )}
    </span>
  );
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
// the state at the end of the line.
function kindKey(item: WorklistItem): MessageKey {
  return item.email_summary ? "timeline.kind.email" : eyebrowKeyFor(item);
}

function kindIcon(item: WorklistItem): LucideIcon {
  return item.email_summary ? Mail : CATEGORY_ICONS[item.category];
}

type CardStatus = Readonly<{ key: MessageKey; tone: "danger" | "warning" }>;

/** Late first, then why a message is waiting; any other row has no state. */
function statusOf(item: WorklistItem, titled: boolean): CardStatus | null {
  if (item.overdue === true)
    return titled ? null : { key: "worklist.overdue", tone: "danger" };
  if (item.email_summary && item.band === "now")
    return { key: eyebrowKeyFor(item), tone: "warning" };
  return null;
}

/**
 * The clock a card races. An undated task has none, so it shows none. A late
 * one says its day and how late it is, the two facts a reader acts on.
 */
export function cardWhen(
  item: WorklistItem,
  words: Words,
  zone: string,
  recordZone: string,
  now: Date,
): string | null {
  if (item.source === "task" && !item.due_at) return null;
  const { t, plural, locale } = words;
  if (item.overdue === true && item.due_at) {
    const clock = item.source === "task" ? recordZone : zone;
    const late = calendarDaysBetween(new Date(item.due_at), now, clock);
    if (late > 0)
      return [
        formatDayMonthNearYear(item.due_at, locale, clock, now),
        plural("co.next.daysLate", late, { days: formatNumber(late, locale) }),
      ].join(" · ");
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

const SUBJECT_ICONS: Partial<Record<string, LucideIcon>> = {
  deal: Handshake,
  lead: UserPlus,
  project: FolderKanban,
};

/**
 * Who the work is with and what it is about. That is the contact, then their
 * account, then the row's record where it is neither.
 */
function TaskRecords({
  item,
  link,
}: Readonly<{ item: WorklistItem; link?: (href: string) => string }>) {
  // A brief item's open card is headed by its record, so no chip repeats it.
  const headed = link !== undefined && briefSubjectLabel(item) !== undefined;
  const named = namedOn(item).filter(
    (who, index, all) =>
      all.findIndex((other) => other.id === who.id) === index &&
      !(headed && who.id === item.subject?.id),
  );
  return named.map((who) => {
    const appHref = link && who.href ? link(who.href) : undefined;
    return who.mark === "avatar" ? (
      <Chip
        key={who.id}
        dense
        appHref={appHref}
        lead={<Avatar name={who.label} identity={who.id} />}
      >
        {who.label}
      </Chip>
    ) : (
      <Chip key={who.id} dense appHref={appHref} icon={who.mark}>
        {who.label}
      </Chip>
    );
  });
}

/** The subject has a chip of its own on the card, so the name need not say it. */
function subjectChipped(item: WorklistItem): boolean {
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
