// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A `day` activity knows its date and no time. Its time reaches a reader in a
// zone only through activitywhen.ts, which prints its UTC date and no clock.
//
// A reader in a zone is derived from the code, never listed. It is the format
// call of an Intl.DateTimeFormat, or a Date's toLocale*String. In format/ it is
// also a function that puts an hour or minute count into its sentence, or
// builds a RelativeTimeFormat.
//
// The value is followed through calls, props, fields, object literals,
// bindings, assignments, `.map` callbacks and element reads. It is followed
// through `new Date`, `Date.parse`, `Math`, arithmetic and template text too.
// A call returning a boolean hands it nowhere.
//
// A `due_at` is an activity time only on a row that can be a meeting. A
// condition on the same row's `source` that rules the meeting out ends it.
//
// A field `x_at` is an activity time too where its record carries
// `x_precision`. A precision handed to the owner must trace to a wire
// precision. Or it is `instant` for an `x_precision` the wire left out, handed
// beside that record's own `x_at`. Any other literal passes only when the time
// it is paired with reads no activity time.
//
// Not followed, each a known blind spot:
// - a value kept in a Map or pushed into an array, and read back;
// - ISO text cut by hand, as in `at.slice(0, 10)`;
// - a Date's own getters, as in `getDate()`;
// - a comparison against now or another day, which the owner's predicates answer;
// - a projection that carries an activity time without its precision.

import type * as Sweep from "../../scripts/lib/activity-precision.sweep";
import {
  type Census,
  census,
  shipped,
} from "../../scripts/lib/activity-precision.sweep";
import { inWorker } from "../../scripts/lib/in-worker";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

// The formatters the derivation must arrive at, as proof that it ran.
export const DERIVED_READERS = [
  "formatDate",
  "formatDayMonth",
  "formatDateTime",
  "formatTimeOfDay",
  "formatTimeRange",
  "formatRelativeTime",
  "formatElapsed",
  "calendarDay",
];

const inScope = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") && shipped(file.path);

// Floors and findings read one census, typed once per program.
const SWEEP = new URL(
  "../../scripts/lib/activity-precision.sweep.ts",
  import.meta.url,
);

/** The census over the corpus, typed in a worker thread of its own. */
const verdictOf = (corpus: readonly CensusFile[]): Promise<Census> =>
  inWorker<typeof Sweep, "sweepFiles">(
    SWEEP,
    "sweepFiles",
    corpus.map((file) => file.path),
  );

/**
 * The same census in this thread, over the tree's shared program, for a
 * planted tree whose files exist only in memory.
 */
export const judge = (corpus: readonly CensusFile[], tree: CensusTree) =>
  verdictFindings(
    census(
      tree.checked(),
      corpus.map((file) => file.path),
    ),
  );

/** What the census says, its derivation's own floors first. */
export function verdictFindings({
  findings,
  unproven,
  unjudged,
  readers,
}: Census): string[] {
  const missing = DERIVED_READERS.filter((name) => !readers.has(name));
  const say = (what: string) => (site: Census["findings"][number]) =>
    `${site.file}:${site.line}: ${what}: ${site.text}`;
  return [
    ...(missing.length === 0
      ? []
      : [`the derivation missed the formatters ${missing.join(", ")}`]),
    ...(readers.has("formatNumber")
      ? ["the derivation took formatNumber for a reader in a zone"]
      : []),
    ...findings.map(say("a day entry's clock is drawn")),
    ...unproven.map(say("a precision the owner cannot trace")),
    ...unjudged.map(say("an activity time the census could not judge")),
  ];
}

// The top folder of src/ a file sits in, or undefined outside src/.
const srcFolder = (file: CensusFile) =>
  file.area === "src" ? file.rel.split("/")[1] : undefined;

export const rule: CensusRule = {
  name: "activity-precision",
  corpus: (tree) => tree.files.filter(inScope),
  floors: (corpus) => {
    const folders = new Set(
      corpus.map(srcFolder).filter((name) => name?.includes(".") === false),
    );
    return [
      ...(folders.size > 3
        ? []
        : ["the census reached three src/ folders or fewer"]),
      ...(corpus.some((file) => file.area === "extensions")
        ? []
        : ["the census reached no extension frontend layer"]),
    ];
  },
  settle: async (corpus) => verdictFindings(await verdictOf(corpus)),
  advice:
    "An activity time reaches a reader through formatActivityWhen, activityAgo " +
    "or activityDay with its own precision, so a day entry prints its date and " +
    "no clock. Hand the owner the wire precision beside its own time.",
};
