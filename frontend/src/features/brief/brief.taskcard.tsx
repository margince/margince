// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One row of the day as a to-do card: the tick, what to do, and who it is
// with. The row in hand is drawn whole by brief.focus.tsx; these are the rest.

import { CalendarDays, CircleCheck } from "lucide-react";
import { Badge, Card } from "../../design-system/atoms";
import { Avatar } from "../../design-system/avatar";
import { IconAction } from "../../design-system/iconaction";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { useMe } from "../../lib/session";
import {
  briefContextTitle,
  briefSubjectLabel,
  dealFactsText,
  itemTitle,
  phrasedReasons,
  reasonText,
  whenText,
} from "../../lib/worklistcopy";
import { eyebrowKeyFor } from "../../lib/worklisteyebrow";
import type { WorklistItem } from "../worklist/worklist.queries";
import { readerTask } from "../worklist/worklist.reader";
import { useTaskDone } from "../worklist/worklist.row.answer";

/** The name a card goes by, the way the row in hand names it. */
export function taskName(
  item: WorklistItem,
  viewer: Parameters<typeof readerTask>[1],
  t: Translator,
  locale: Locale,
): string {
  return (
    briefSubjectLabel(item) ?? itemTitle(readerTask(item, viewer, t), t, locale)
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
      icon={<CircleCheck aria-hidden="true" />}
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
  const recordZone = useRecordZone();
  const viewer = useMe(false).data?.user;
  const when = cardWhen(item, t, locale, zone, recordZone);
  const line = taskLine(item, t, locale, zone);
  // Deals that read alike stay apart by their figures.
  const facts = item.deal ? dealFactsText(item, t, locale, zone) : null;
  return (
    <Card as="article" className="brief-task">
      <TaskTick item={item} />
      <button type="button" className="brief-task-open" onClick={onOpen}>
        <span className="brief-task-title">
          {taskName(item, viewer, t, locale)}
        </span>
        {line && <span className="brief-task-line">{line}</span>}
        <span className="brief-task-meta">
          {when && (
            <Badge tone={dueTone(item)} icon={CalendarDays}>
              {when}
            </Badge>
          )}
          <span className="t-caption">{t(eyebrowKeyFor(item))}</span>
          <TaskPeople item={item} />
          {facts && <span className="t-caption t-num">{facts}</span>}
        </span>
      </button>
    </Card>
  );
}

/** The clock a card races. An undated task has none, so it shows none. */
export function cardWhen(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
  recordZone: string,
): string | null {
  if (item.source === "task" && !item.due_at) return null;
  return whenText(item, t, locale, zone, recordZone, new Date());
}

/** Late is danger, the day's first band a warning, anything else plain. */
export function dueTone(item: WorklistItem): "danger" | "warning" | "default" {
  if (item.overdue === true) return "danger";
  return item.band === "now" ? "warning" : "default";
}

/** Who the work is with and what it is about, as a reader knows them. */
export function TaskPeople({ item }: Readonly<{ item: WorklistItem }>) {
  const about =
    item.subject?.id === item.contact?.id ? undefined : item.subject;
  const named = [
    item.contact && { ...item.contact, type: "contact" as const },
    about,
  ].filter((who) => who?.label !== undefined && who.label !== "");
  return named.map((who) =>
    who ? (
      <span key={who.id} className="brief-task-who">
        {(who.type === "contact" || who.type === "company") && (
          <Avatar name={who.label ?? ""} identity={who.id} />
        )}
        {who.label}
      </span>
    ) : null,
  );
}
