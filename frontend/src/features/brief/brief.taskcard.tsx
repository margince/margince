// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One row of the day as a to-do card: the tick, what to do, and who it is
// with. The row in hand is drawn whole by brief.focus.tsx; these are the rest.

import { Circle } from "lucide-react";
import { MayBeDone } from "../../components/maybedone";
import { useTaskDone } from "../../components/taskactions";
import { subjectChipped, TaskMeta } from "../../components/taskmeta";
import { Card } from "../../design-system/atoms";
import { IconAction } from "../../design-system/iconaction";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { changeSentence } from "../../lib/calendarchange";
import { useMe } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import { worklistKey } from "../../lib/workkeys";
import {
  briefContextTitle,
  briefSubjectLabel,
  itemTitle,
  partyName,
  phrasedReasons,
  readerTask,
  reasonText,
  sentenceTitled,
} from "../../lib/worklistcopy";
import { eventLine, useHappenedClock } from "../../lib/worklisthappened";
import type { WorklistItem } from "../../lib/worklisttypes";

/** The name a card goes by, the way the row in hand names it. */
export function taskName(
  item: WorklistItem,
  viewer: Parameters<typeof readerTask>[1],
  t: Translator,
  locale: Locale,
): string {
  const mail = item.email_summary;
  const sender = partyName(item);
  // A message waiting on the reader is named by the move it asks for.
  // An invitation's subject already names the event it asks about.
  // A later calendar mail is named for its meeting, as the box draws it.
  if (mail?.calendar_change)
    return mail.calendar_change.title ?? mail.subject ?? t("email.noSubject");
  if (mail?.calendar_invitation && mail.subject) return mail.subject;
  if (mail?.move === "needs_reply" && sender && mail.subject)
    return t("brief.task.reply", { name: sender, subject: mail.subject });
  return (
    briefSubjectLabel(item) ??
    itemTitle(
      readerTask(sentenceTitled(item), viewer, t),
      t,
      locale,
      subjectChipped(item),
    )
  );
}

/**
 * The line under a card's name. The agent's own move first, then the row's
 * event, then the first reason it is here, then the producer's own line.
 * Null when the row has none of them: the kind's stock sentence on every card
 * would be one line said six times.
 */
export function taskLine(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
  event: string | null = null,
): string | null {
  // A meeting's change is the line: what moved, or who answered.
  const change = item.email_summary?.calendar_change;
  if (change) return changeSentence(change, t, locale, zone);
  // The meta row's badges carry the clock, so the line leaves it out.
  const reason = phrasedReasons(item, true)
    .map((entry) => reasonText(entry, t, locale, zone, partyName(item)))
    .find((text) => text !== null);
  return briefContextTitle(item) ?? event ?? reason ?? (item.detail || null);
}

/** Done on a row that is not a task: the queue's put-down, handed in. */
export type RowDone = Readonly<{
  offered: boolean;
  pending: boolean;
  done: () => void;
}>;

/**
 * The tick that marks a card done. A task the reader may complete closes the
 * task; any other row the reader may mark done is put down in the queue.
 * A card with neither keeps the column empty, so every name starts on one x.
 */
export function TaskTick({
  item,
  rowDone,
}: Readonly<{ item: WorklistItem; rowDone?: RowDone }>) {
  const completes =
    item.source === "task" && !item.batch && item.actions.includes("complete");
  return (
    <span className="brief-task-tick">
      {completes ? (
        <TaskDone item={item} />
      ) : (
        rowDone?.offered && (
          <Tick item={item} pending={rowDone.pending} onDone={rowDone.done} />
        )
      )}
    </span>
  );
}

function TaskDone({ item }: Readonly<{ item: WorklistItem }>) {
  const done = useTaskDone(item.id, item.version);
  return (
    <Tick
      item={item}
      pending={done.pending}
      onDone={() => void done.complete()}
    />
  );
}

function Tick({
  item,
  pending,
  onDone,
}: Readonly<{ item: WorklistItem; pending: boolean; onDone: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const viewer = useMe(false).data?.user;
  return (
    <IconAction
      label={t("brief.task.done", {
        title: taskName(item, viewer, t, locale),
      })}
      icon={<Circle aria-hidden="true" />}
      variant="ghost"
      pending={pending}
      onClick={onDone}
    />
  );
}

/** A row still waiting behind the one in hand; a press puts it in hand. */
export function TaskCard({
  item,
  onOpen,
  rowDone,
}: Readonly<{ item: WorklistItem; onOpen: () => void; rowDone?: RowDone }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const viewer = useMe(false).data?.user;
  const clock = useHappenedClock();
  const line = taskLine(item, t, locale, zone, eventLine(item, t, clock));
  return (
    <Card as="article" className="brief-task">
      <TaskTick item={item} rowDone={rowDone} />
      <button type="button" className="brief-task-open" onClick={onOpen}>
        <span className="brief-task-title">
          {taskName(item, viewer, t, locale)}
        </span>
        {line && <span className="brief-task-line">{line}</span>}
        <TaskMeta item={item} />
      </button>
      {item.may_be_done && (
        <div className="brief-task-ask">
          <MayBeDone
            question={item.may_be_done}
            task={{
              id: item.id,
              version: item.version,
              canComplete: item.actions.includes("complete"),
            }}
            refresh={[worklistKey]}
          />
        </div>
      )}
    </Card>
  );
}
