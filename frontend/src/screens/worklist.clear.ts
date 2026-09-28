import type { Translator } from "../i18n";
import type { WorklistFilter } from "./worklist.queries";

// The day, drawn.
//
// TWO kinds of number reach this component and they must not be confused.
// `day` is the first page: its summary, counts, reach and scope options
// describe the whole assembled day and do not move as the reader pages.
// `queue` is every row loaded so far, which grows. Reading rows off `day`
// would draw only the first page; reading figures off the latest page would
// describe a slice as though it were the day.
// Which "there is nothing here" sentence an empty queue earns.
//
// A partial read outranks the rest: a day cannot be reported clear while
// something that would have filled it was never read. Then the Tasks pill names
// its HORIZON — this queue is today's, so a task due tomorrow is deliberately
// absent, and "Nothing is waiting on you" read as "you have no work" to a rep
// looking at three open tasks on the company page beside it. The other pills
// keep the unqualified sentence: the full queue carries replies and reviews
// that have no deadline, so "due today" would be the wrong frame for it.
//
// WHOSE day is clear is the last question, and only the unqualified sentence
// gets it wrong: it is the one arm that says "on YOU", and on a colleague's
// queue that named the reader over somebody else's empty day. The other two
// describe the READ rather than the reader and stay as they are.
//
// The name is the roster's, and its absence falls back to the unqualified
// sentence rather than to an id or a gap: a reader who cannot be named is a
// question this line does not have to answer, and "Nothing is waiting on
// 4f3c…" is worse than a sentence one word too general.
export function clearSentence(
  partial: boolean,
  filter: WorklistFilter,
  colleague: string | null,
  t: Translator,
): string {
  if (partial) {
    return t("worklist.clearOfWhatWasRead");
  }
  if (filter === "tasks") {
    return t("worklist.clearOfTasksToday");
  }
  return colleague
    ? t("worklist.clearFor", { name: colleague })
    : t("worklist.clear");
}
