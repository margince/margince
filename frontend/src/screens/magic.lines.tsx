// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One line of the receipt: what happened, why and what it means, then the
// record it is about, who acted and when, with the way back beside it.

import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import { PanelBody, PanelGroupHead } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import {
  magicByKey,
  magicConsequenceKey,
  magicSentenceKey,
  magicWhyKey,
} from "./magic.keys";
import type { MagicLane, MagicLine } from "./magic.queries";
import { LineRecordsOpener } from "./magic.records";
import { MagicUndoButton } from "./magic.undo";

/** One lane with lines in it: its heading, then each line. */
export function MagicLaneSection({
  lane,
  title,
  rows,
  since,
  zone,
}: Readonly<{
  lane: MagicLane;
  title: string;
  rows: readonly MagicLine[];
  since: string | undefined;
  zone: string;
}>) {
  return (
    <>
      <PanelGroupHead title={title} level="h3" />
      <PanelBody>
        <ul className="magic-lines" aria-label={title}>
          {rows.map((row) => (
            // The lanes mint their ids independently, so an id alone can name a
            // row in a lane the reader was not looking at.
            <li className="magic-line" key={`${lane}-${row.id}`}>
              <div className="magic-line-text">
                <LineSentence line={row} />
                <LineMeta line={row} since={since} zone={zone} />
              </div>
              <div className="magic-line-back">
                <LineWayBack line={row} />
              </div>
            </li>
          ))}
        </ul>
      </PanelBody>
    </>
  );
}

/**
 * The record the line is about, who acted, and when: each its own element, so
 * the separators between them are the stylesheet's and never part of a value.
 */
function LineMeta({
  line,
  since,
  zone,
}: Readonly<{ line: MagicLine; since: string | undefined; zone: string }>) {
  const t = useT();
  const by = line.actor.label ? magicByKey(line.actor.label.key) : null;
  return (
    <p className="t-caption magic-line-meta">
      <span>
        <LineSubject line={line} since={since} />
      </span>
      {by && <span>{t(by, line.actor.label?.values)}</span>}
      <span>
        <LineWhen line={line} zone={zone} />
      </span>
    </p>
  );
}

/**
 * What happened, and what it means for the reader.
 *
 * A sentence this build has no key for is DROPPED rather than printed: a newer
 * server mints keys this client predates, and `magic.action.something` on a
 * receipt is worse than a row that says only what it was about and when.
 */
function LineSentence({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  const sentence = magicSentenceKey(line.summary.key);
  const consequence = line.consequence
    ? magicConsequenceKey(line.consequence)
    : null;
  // WHY the machinery did it, under what it did. A reason key this build
  // predates draws nothing, for the reason an unknown sentence does.
  const why = line.reason ? magicWhyKey(line.reason.key) : null;
  return (
    <>
      {sentence && <span>{t(sentence, line.summary.values)}</span>}
      {why && line.reason && (
        <p className="t-caption">{t(why, line.reason.values)}</p>
      )}
      {consequence && <p className="t-caption">{t(consequence)}</p>}
    </>
  );
}

/**
 * When it happened, and for a watched source how long it has been that way.
 *
 * A watching line's occurred_at is when the condition was OBSERVED, so printing
 * it alone would date every outage to this page load. Where the condition has a
 * beginning the server sends it, and that is the figure a reader acts on.
 */
function LineWhen({ line, zone }: Readonly<{ line: MagicLine; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const began = line.summary.values?.failing_since;
  if (line.lane === "watching" && began) {
    return t("magic.failingSince", {
      when: formatDateTime(began, locale, zone),
    });
  }
  return formatDateTime(line.occurred_at, locale, zone);
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
  // A done line about records OPENS: to every record it stands for, what
  // changed on each, and an undo per record. Retention names no record, and a
  // line from another lane is not a change to inspect.
  const opens = line.lane === "done" && line.entity !== undefined && since;
  // ONE line for a job that touched many records: the most recent one by
  // name, and how many more.
  if (count > 1 || (!line.entity && line.count !== undefined)) {
    const summary =
      label && count > 1
        ? plural("magic.aboutMany", count - 1, {
            label,
            others: formatNumber(count - 1, locale),
          })
        : plural("magic.aboutCount", count, {
            count: formatNumber(count, locale),
          });
    return opens ? (
      <LineRecordsOpener line={line} since={since} summary={summary} />
    ) : (
      summary
    );
  }
  if (!label) {
    return t("magic.noRecord");
  }
  const href = recordHref(line);
  return (
    <>
      {href ? <a href={href}>{label}</a> : label}
      {opens && (
        <LineRecordsOpener
          line={line}
          since={since}
          summary={t("magic.records.show")}
        />
      )}
    </>
  );
}

/**
 * The way back: an undo for a change to one record, done here with one press.
 * A line standing for many records offers its undos inside, one per record —
 * one button that put back 150 changes nobody had looked at would be the same
 * unasked bulk write this page exists to report.
 */
function LineWayBack({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  if ((line.count ?? 1) > 1 && line.entity) {
    return <span className="t-caption">{t("magic.undo.perRecord")}</span>;
  }
  // A line naming no record still says why it cannot be taken back; it offers
  // no press, because the restore route needs a record to name.
  return (
    <MagicUndoButton
      undo={line.undo}
      entityType={line.entity?.type ?? ""}
      entityId={line.entity?.id ?? ""}
    />
  );
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
