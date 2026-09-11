// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Turning one occurrence into the sentence the rail says, and the two questions
// that decide whether it says anything at all: is this a kind this build knows,
// and does the copy table have words for the state it is in.
//
// The words themselves live in ai-activity-lines.ts. This file is what reads
// them — kept apart because the copy table is the half that grows with every
// kind, and the reading of it has not changed since the rail shipped.

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { ACTIVITY_LINE, type LineSet, NAMED_LINE } from "./ai-activity-lines";
import { ENTITY, isEntityKind } from "./entity";
import type { Route } from "./router";

type ActivityKind = components["schemas"]["AiActivityItem"]["kind"];

/**
 * One line as the rail says it: the words, and where the record's name sits in
 * them.
 *
 * Three pieces rather than one string because the name is not only a word in
 * the sentence — it is the way to the record, and a link can only be drawn
 * around a part the caller can still tell from the rest. `before` alone is
 * the whole line when it names no record.
 */
export type SpokenLine = Readonly<{
  before: string;
  /**
   * The record the line is about, with its page when this build has one for
   * that kind of record. A name with no route is drawn as text: a document or
   * a meeting has no page of its own, and so does a kind a newer server named
   * that this tab has never heard of.
   */
  subject: Readonly<{ name: string; route: Route | null }> | null;
  after: string;
}>;

/** A line that names no record: words and nothing else. */
export function plain(words: string): SpokenLine {
  return { before: words, subject: null, after: "" };
}

/** The line as one string, for a surface that draws no link. */
export function spokenText(line: SpokenLine): string {
  return `${line.before}${line.subject?.name ?? ""}${line.after}`;
}

/**
 * What to say about one item, or nothing at all.
 *
 * The existence check is not optional and `t()` cannot do it: translate() falls
 * back to THE KEY STRING, so a missing entry would put
 * `agent.activity.foo.running` in front of a reader.
 *
 * There are three ways to draw nothing, and they are different facts: the kind
 * is one this build has never heard of, the kind is one it deliberately does
 * not narrate, or the state has no line under a kind it does narrate. All three
 * answer null, because the reader's screen is the same either way — but they
 * are distinguished HERE rather than collapsed into a lookup miss, so a kind
 * that silently lost its copy cannot hide among the ones that never had any.
 *
 * It takes RAW strings rather than the contract's unions, and that widening is
 * the point: the map is total over the contract, so the only way to reach the
 * first case is a value the contract does not carry — which is exactly what an
 * older tab gets from a newer server that has added a kind or a state. Typed
 * narrowly, that case could only be written with a cast, and a test that casts
 * is asserting against its own escape hatch instead of against the function.
 */
export function speak(
  item: Readonly<{
    kind: string;
    state: string;
    subject_label?: string | null;
    subject_type?: string | null;
    subject_id?: string | null;
  }>,
  t: (key: MessageKey, params?: Record<string, string>) => string,
): SpokenLine | null {
  if (!isActivityKind(item.kind)) {
    return null;
  }
  const entry = ACTIVITY_LINE[item.kind];
  if ("notDisplayed" in entry) {
    return null;
  }
  // The named line first, and ONLY when both halves are present: a kind that
  // has named copy and an occurrence that carried a name. Either missing falls
  // through to the unnamed sentence, which is why an absent label is an
  // ordinary case here rather than a gap to report.
  const name = item.subject_label ?? "";
  if (name !== "") {
    const named: Readonly<Partial<Record<string, MessageKey>>> =
      NAMED_LINE[item.kind] ?? {};
    const namedKey = named[item.state];
    if (namedKey !== undefined) {
      return aroundTheName(t(namedKey), name, subjectRoute(item));
    }
  }
  const byState: Readonly<Partial<Record<string, MessageKey>>> = entry;
  const key = byState[item.state];
  return key === undefined ? null : plain(t(key));
}

/**
 * The placeholder a named line is written around, in every locale.
 *
 * Held to exactly this spelling by ai-activity-lines.test.ts, which is what
 * lets the sentence be split here rather than interpolated: translate() would
 * put the name IN the words, and a name already in the words cannot be told
 * from them again — a company called "I" would link the wrong word.
 */
const NAME_PLACEHOLDER = "{name}";

/**
 * The translated template with the name set in its own slot.
 *
 * A template without the slot is one the copy gate does not admit, so the
 * branch is a guard against a fake translator rather than a case the catalog
 * can reach; it keeps the words and drops nothing.
 */
function aroundTheName(
  template: string,
  name: string,
  route: Route | null,
): SpokenLine {
  const at = template.indexOf(NAME_PLACEHOLDER);
  if (at < 0) {
    return plain(template);
  }
  return {
    before: template.slice(0, at),
    subject: { name, route },
    after: template.slice(at + NAME_PLACEHOLDER.length),
  };
}

/**
 * The page the occurrence's record has, or null when it has none.
 *
 * `isEntityKind` is the same question every other link in the app asks before
 * it links: the wire's type is free text — a document is `attachment`, a
 * meeting is `activity`, a newer server may say something this build has never
 * seen — and only the kinds with a record page become a route. Both halves have
 * to be present; a type with no id is nowhere to go.
 */
function subjectRoute(
  item: Readonly<{ subject_type?: string | null; subject_id?: string | null }>,
): Route | null {
  const type = item.subject_type ?? "";
  const id = item.subject_id ?? "";
  if (id === "" || !isEntityKind(type)) {
    return null;
  }
  return ENTITY[type].route(id);
}

/**
 * Whether a raw string is a kind this build knows.
 *
 * An own-key check rather than a cast, so the narrowing is something the
 * runtime actually did: a newer server's kind is not in the map, and saying so
 * is the whole answer speak gives for it.
 */
function isActivityKind(kind: string): kind is ActivityKind {
  return Object.hasOwn(ACTIVITY_LINE, kind);
}

/**
 * The kinds this rail draws, as the server's `kinds` filter takes them.
 *
 * DERIVED from ACTIVITY_LINE rather than listed, and that is the whole point:
 * the server reports every AI task, and `recent` is bounded at ten. A client
 * that asked for everything and drew three kinds would be handed the newest ten
 * of twenty-three — ten it renders nothing for — and the rail would go blank on
 * the day a rep used the composer a lot, while the projection was right the
 * whole time. Naming the kinds moves the bound inside this list; deriving it
 * means the list cannot fall out of step with the copy that decides it.
 */
export function displayedKinds(): ActivityKind[] {
  return displayedLines().map(([kind]) => kind);
}

/**
 * The narrated kinds paired with their line tables.
 *
 * The narrowing happens HERE, once, where `"notDisplayed" in entry` is a check
 * the compiler performs rather than an assertion somebody makes: a caller that
 * looked the entry up again would be holding `LineSet | NotDisplayed` and would
 * need a cast to say what it already knows. Returning the pair means nobody
 * downstream has to.
 *
 * `Object.entries` widens the key to `string`, so the entry list is rebuilt from
 * the map's own keys rather than trusting that widening back — `ACTIVITY_LINE`
 * is a total `Record<ActivityKind, …>`, so its keys ARE the kinds.
 */
export function displayedLines(): [ActivityKind, LineSet][] {
  const kinds = Object.keys(ACTIVITY_LINE) as ActivityKind[];
  return kinds.flatMap((kind) => {
    const entry = ACTIVITY_LINE[kind];
    return "notDisplayed" in entry
      ? []
      : [[kind, entry] as [ActivityKind, LineSet]];
  });
}
