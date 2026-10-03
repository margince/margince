// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One line of the receipt, on one row: its mark in the clock's own colours,
// what happened and the record it is about, then the undo where there is one
// and the time, in columns the whole list shares. Who acted, why, and why a
// change cannot be put back are the line's detail: on hover, and read out.

import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import { Disclosure } from "../design-system/atoms";
import { PanelBody, PanelGroupHead } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import type { Locale } from "../i18n/locale";
import { approvalHref } from "./approvaldrawer";
import { approvalKindLabel } from "./approvalkind";
import {
  magicByKey,
  magicConsequenceKey,
  magicSentenceKey,
  magicUndoReasonKey,
  magicWhyKey,
} from "./magic.keys";
import type { MagicLane, MagicLine } from "./magic.queries";
import { LineRecordsOpener } from "./magic.records";
import { axisLabel, markKind } from "./magic.timeline";
import { MagicUndoButton, undoPress } from "./magic.undo";

/**
 * One lane with lines in it: its heading, then each line, folded under `fold`
 * where the panel already said it at a glance.
 */
export function MagicLaneSection({
  lane,
  title,
  rows,
  since,
  zone,
  byDay,
  fold,
}: Readonly<{
  lane: MagicLane;
  title: string;
  rows: readonly MagicLine[];
  since: string | undefined;
  zone: string;
  // The window runs past a day, so a line names its day rather than its hour.
  byDay: boolean;
  fold?: string;
}>) {
  const lines = (
    <ul className="magic-lines" aria-label={title}>
      {rows.map((row) => (
        // The lanes mint their ids independently, so an id alone can name a
        // row in a lane the reader was not looking at.
        <MagicLineRow
          key={`${lane}-${row.id}`}
          line={row}
          since={since}
          zone={zone}
          byDay={byDay}
        />
      ))}
    </ul>
  );
  return (
    <>
      <PanelGroupHead title={title} level="h3" />
      <PanelBody>
        {fold ? (
          <Disclosure summary={fold} className="magic-fold">
            {lines}
          </Disclosure>
        ) : (
          lines
        )}
      </PanelBody>
    </>
  );
}

function MagicLineRow({
  line,
  since,
  zone,
  byDay,
}: Readonly<{
  line: MagicLine;
  since: string | undefined;
  zone: string;
  byDay: boolean;
}>) {
  const t = useT();
  const sentence = magicSentenceKey(line.summary.key);
  const detail = lineDetail(line, t);
  return (
    <li className="magic-line" title={detail.join(" · ") || undefined}>
      {/* A watching line is off the clock, so its mark is its lane's. */}
      <span
        className="magic-mark"
        data-kind={markKind(line) ?? "restore"}
        aria-hidden="true"
      />
      <p className="magic-line-text">
        {/* A sentence this build has no key for is DROPPED rather than
            printed: `magic.action.something` on a receipt is worse than a row
            that says only what it was about and when. */}
        {sentence && t(sentence, readableValues(line.summary.values, t))}{" "}
        <LineSubject line={line} since={since} />
        {detail.length > 0 && (
          <span className="sr-only">
            {detail.map((part) => (
              <span key={part}>{part} </span>
            ))}
          </span>
        )}
      </p>
      <LineUndo line={line} />
      <LineWhen line={line} zone={zone} byDay={byDay} />
    </li>
  );
}

/**
 * Who acted, why, what it means, and for a done change that cannot be put
 * back, why not: true of the line and not needed to scan it, so it is the
 * row's tooltip and is read out rather than printed under every line. A key
 * this build predates says nothing, for the reason an unknown sentence does.
 */
function lineDetail(line: MagicLine, t: ReturnType<typeof useT>): string[] {
  const by = line.actor.label ? magicByKey(line.actor.label.key) : null;
  const why = line.reason ? magicWhyKey(line.reason.key) : null;
  const consequence = line.consequence
    ? magicConsequenceKey(line.consequence)
    : null;
  return [
    by && t(by, line.actor.label?.values),
    why && t(why, line.reason?.values),
    consequence && t(consequence),
    noWayBack(line, t),
  ].filter((part): part is string => Boolean(part));
}

// Why a done change to one record cannot be put back, said rather than shown as
// a greyed control. A line standing for many says it per record inside, and on
// the other lanes nothing changed, so "nothing to put back" would be noise.
function noWayBack(line: MagicLine, t: ReturnType<typeof useT>): string | null {
  const entity = line.entity;
  if (line.lane !== "done" || (standsForMany(line) && entity)) {
    return null;
  }
  if (undoPress(line.undo, entity?.type ?? "", entity?.id ?? "")) {
    return null;
  }
  const reason = magicUndoReasonKey(line.undo?.reason);
  return reason ? t(reason) : null;
}

/**
 * The undo for a done change to one record, the one control a line carries.
 * A line standing for many offers its undos inside, one per record: one press
 * that put back 150 changes nobody had looked at would be the same unasked
 * bulk write this page exists to report.
 *
 * A decision waiting has nothing to undo. Its control is the way to the
 * worklist, where every staged decision is answered.
 */
function LineUndo({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  if (line.lane === "needs_you") {
    return (
      <div className="magic-line-controls">
        <a href={approvalHref(line.id)}>{t("magic.decide")}</a>
      </div>
    );
  }
  const entity = line.entity;
  if (
    line.lane !== "done" ||
    !entity ||
    standsForMany(line) ||
    !undoPress(line.undo, entity.type, entity.id)
  ) {
    return null;
  }
  return (
    <div className="magic-line-controls">
      <MagicUndoButton
        undo={line.undo}
        entityType={entity.type}
        entityId={entity.id}
      />
    </div>
  );
}

/**
 * When it happened, and for a watched source how long it has been that way.
 *
 * A watching line's occurred_at is when the condition was OBSERVED, so printing
 * it alone would date every outage to this page load. Where the condition has a
 * beginning the server sends it, and that is the figure a reader acts on.
 */
function LineWhen({
  line,
  zone,
  byDay,
}: Readonly<{ line: MagicLine; zone: string; byDay: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const began = line.summary.values?.failing_since;
  if (line.lane === "watching" && began) {
    return (
      <span className="t-caption magic-line-when">
        {t("magic.failingSince", { when: formatDateTime(began, locale, zone) })}
      </span>
    );
  }
  // Short, as the clock's axis spells it; the full instant is one hover away.
  return (
    <time
      className="t-caption magic-line-when"
      dateTime={line.occurred_at}
      title={formatDateTime(line.occurred_at, locale, zone)}
    >
      {axisLabel(line.occurred_at, byDay, locale, zone)}
    </time>
  );
}

/**
 * How a line that stands for more than the record it names says so.
 *
 * `floor` is the receipt's own warning that the read behind the count was cut
 * short, so every wording it picks is an "at least": the exact one would
 * understate how far a machine went, which is the direction a reader of this
 * page cannot recover from.
 */
function manySummary({
  t,
  plural,
  locale,
  label,
  count,
  floor,
}: Readonly<{
  t: ReturnType<typeof useT>;
  plural: ReturnType<typeof usePlural>;
  locale: Locale;
  label: string | undefined;
  count: number;
  floor: boolean;
}>) {
  if (label && count > 1) {
    return plural(
      floor ? "magic.aboutManyAtLeast" : "magic.aboutMany",
      count - 1,
      { label, others: formatNumber(count - 1, locale) },
    );
  }
  if (label && floor) {
    return t("magic.aboutNamedAtLeast", { label });
  }
  return plural(floor ? "magic.aboutCountAtLeast" : "magic.aboutCount", count, {
    count: formatNumber(count, locale),
  });
}

/**
 * Which record the line is about, as a link where the app has a page for it.
 *
 * The subject is its own value rather than a word inside the sentence:
 * interpolating a record's name into a translated clause is how a sentence
 * ends up ungrammatical in two of three languages.
 */
function LineSubject({
  line,
  since,
}: Readonly<{ line: MagicLine; since: string | undefined }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const label = subjectLabel(line);
  const count = line.count ?? 1;
  // ONE line for a job that touched many records: the most recent one by
  // name, and how many more.
  // The read behind a line can be cut short, and then its count is the most
  // that read could see rather than what the job did. A floor line therefore
  // never renders as an exact number — including the one that stands for a
  // single record, which is a group that MIGHT have more beyond the cut.
  const floor = line.count_is_floor === true;
  const many =
    standsForMany(line) || (!line.entity && line.count !== undefined);
  const shown = many
    ? manySummary({ t, plural, locale, label, count, floor })
    : label;
  // No record to name says nothing: the sentence stands alone.
  if (!shown) {
    return null;
  }
  // A done change about records OPENS, to every record it touched, what
  // changed on each and an undo per record; inside, each links to its page.
  // Retention names no record, and another lane's line changed nothing.
  const opens = line.lane === "done" && line.entity !== undefined && since;
  const href = many ? undefined : recordHref(line);
  return (
    <span className="magic-line-subject">
      {opens ? (
        <LineRecordsOpener line={line} since={since} summary={shown} />
      ) : href ? (
        <a href={href}>{shown}</a>
      ) : (
        shown
      )}
    </span>
  );
}

/**
 * Whether a line stands for more than the record it names. A count cut short
 * is a group however small: the read saw one record and there may be more,
 * so its undo is the per-record list's and never a press on the row. The
 * subject, the row's undo and its "why not" all ask this one question.
 */
function standsForMany(line: MagicLine): boolean {
  return (line.count ?? 1) > 1 || line.count_is_floor === true;
}

/**
 * What to call the record this line is about.
 *
 * The entity's own label first, then the caption the approval was staged under,
 * then the mailbox a connector is failing on. Absent rather than invented: a
 * line naming a record it cannot label still says what happened.
 */
function subjectLabel(line: MagicLine): string | undefined {
  const values = line.summary.values;
  return line.entity?.label ?? values?.target ?? values?.account;
}

// The record's address, through the entity registry rather than a switch
// written here: the record types have route names of their own, and a second
// spelling sends a reader to a page that does not exist. An activity resolves
// to nothing on purpose — it is a timeline entry rather than a record with a
// page.
function recordHref(line: MagicLine): string | undefined {
  const entity = line.entity;
  if (!entity || !isEntityKind(entity.type)) {
    return undefined;
  }
  return routeHash(ENTITY[entity.type].route(entity.id));
}

/**
 * A proposal's kind as the reader names it. The wire carries the kind's code
 * (`capture_counterparty`), which is vocabulary, not a sentence.
 */
function readableValues(
  values: Readonly<Record<string, string>> | undefined,
  t: Translator,
): Record<string, string> | undefined {
  if (!values?.kind) return values;
  return { ...values, kind: approvalKindLabel(values.kind, t) };
}
