import type { components } from "../api/schema";
import { calendarDay } from "../format/calendarday";
import { daysPast } from "../format/lateness";

type Activity = components["schemas"]["Activity"];

export type TaskGroup = "overdue" | "today" | "upcoming" | "undated";

// Which bucket a task belongs in, decided in the READER's zone — the same zone
// format/calendarday's `dueInstant` mints the wire instant in. That pairing is
// the point of the two sitting in one module: a bucket read in a different zone
// than the pick was made in files a task under a day the reader never chose.
export function groupTask(task: Activity, now: Date, zone: string): TaskGroup {
  if (!task.due_at) {
    return "undated";
  }
  const due = new Date(task.due_at);
  // Lateness is decided in format/lateness, the one place that answers it for
  // every screen — the bucket boundary and the person card's overdue label are
  // the same question, and they were two spellings that disagreed for a whole
  // day after a task fell due.
  if (daysPast(due.getTime(), now.getTime()).late) {
    return "overdue";
  }
  return calendarDay(due, zone) === calendarDay(now, zone)
    ? "today"
    : "upcoming";
}
