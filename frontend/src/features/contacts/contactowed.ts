import type { components } from "../../api/schema";
import type { Words } from "../../i18n";
import {
  type LatePromise,
  latePromise,
  type PromiseKind,
} from "../../lib/latepromise";

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];
type Activity = components["schemas"]["Activity"];

// What the move says is owed, in the order both readers below take it: the
// promise it may already have settled, then its evidence.
function owedRecords(moment: ContactMoment): { id: string; task: boolean }[] {
  const promise = moment.may_be_done;
  return [
    ...(promise
      ? [{ id: promise.promise_id, task: promise.promise_type === "task" }]
      : []),
    ...moment.evidence.flatMap((item) =>
      item.id ? [{ id: item.id, task: item.type === "task" }] : [],
    ),
  ];
}

// The first listed task the move is about.
export function momentTask(
  moment: ContactMoment,
  view: Contact360,
): Activity | undefined {
  const tasks = view.next_steps?.data ?? [];
  for (const record of owedRecords(moment)) {
    const task = record.task
      ? tasks.find((row) => row.id === record.id)
      : undefined;
    if (task) {
      return task;
    }
  }
  return undefined;
}

// What the move says is owed, as its due date and kind: a claim's or a listed
// task's. A privacy notice is dated like a task.
function owedDue(
  moment: ContactMoment,
  view: Contact360,
): { due: string; kind: PromiseKind } | undefined {
  if (moment.rule === "privacy_notice_due") {
    const due = view.privacy_notice_duties?.find(
      (duty) => duty.id === moment.recommended_action.destination?.entity_id,
    )?.due_at;
    return due ? { due, kind: "task" } : undefined;
  }
  for (const { id } of owedRecords(moment)) {
    const claim = view.claims?.find((row) => row.id === id)?.due_at;
    if (claim) {
      return { due: claim, kind: "claim" };
    }
    const task = view.next_steps?.data.find((row) => row.id === id)?.due_at;
    if (task) {
      return { due: task, kind: "task" };
    }
  }
  return undefined;
}

// How late the owed thing is, against the read's own `as_of`. So the tag agrees
// with the dates beside it and does not drift while a tab is left open.
export function owedLateness(
  moment: ContactMoment,
  view: Contact360,
  zone: string,
  words: Words,
): LatePromise | undefined {
  const owed = owedDue(moment, view);
  return owed && latePromise(owed.due, view.as_of, zone, owed.kind, words);
}

// The records the move is itself about. A task the move names is the subject
// of the bar, and counted again under "+N tasks" it would be owed twice.
export function recordsTheMoveNames(
  move: ContactMoment | undefined,
): Set<string> {
  return new Set(
    (move?.evidence ?? []).flatMap((item) => (item.id ? [item.id] : [])),
  );
}
