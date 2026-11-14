// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The done lane on Home, as the last card of Done for you: the small changes
// Margince made beside the deals. Lines that say the same thing stand as one
// step with a count. What a reader needs only before an undo waits in each
// line's Details.

import { useState } from "react";
import { Badge, Card } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { type Fact, FactList } from "../../design-system/factlist";
import { floorFigure } from "../../format/figure";
import { formatDateTime, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MagicLine } from "../../lib/magic.queries";
import { LineSubject, LineUndo, lineNotes, lineSentence } from "./magic.lines";
import { axisLabel, markKind } from "./magic.marks";

/** Lines whose sentence reads the same, newest first. */
type LineGroup = Readonly<{
  sentence: string;
  lines: readonly MagicLine[];
  records: number;
  floor: boolean;
}>;

export function MagicDoneCard({
  rows,
  since,
  zone,
  byDay,
  floor,
  defaultOpen,
}: Readonly<{
  rows: readonly MagicLine[];
  since: string | undefined;
  zone: string;
  byDay: boolean;
  /** The lane's read was cut short, so its total is only a floor. */
  floor: boolean;
  /** Open on arrival: nothing else in the section asks to be read first. */
  defaultOpen: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // Newest first whatever order the lane arrives in, so a group's time and
  // the card's are its latest change.
  const ordered = [...rows].sort(
    (a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at),
  );
  const groups = groupBySentence(ordered, (line) => lineSentence(line, t));
  const total = groups.reduce((sum, group) => sum + group.records, 0);
  const newest = ordered[0];
  const [open, setOpen] = useState(defaultOpen);
  // One step per kind, as the app's other summary lines list their parts.
  const summary = groups.map((group) => group.sentence).join(" · ");
  return (
    <Card as="article" className="brief-done-deal brief-done-small">
      <Disclosure
        className="brief-done-fold"
        open={open}
        onToggle={setOpen}
        summary={
          <span className="brief-done-head">
            <span className="brief-done-name">
              {plural("brief.done.small", total, {
                count: floorFigure(formatNumber(total, locale), floor, total),
              })}
            </span>
            {!open && newest && (
              <time
                className="t-caption brief-done-latest"
                dateTime={newest.occurred_at}
              >
                {axisLabel(newest.occurred_at, byDay, locale, zone)}
              </time>
            )}
            {!open && <span className="brief-done-net">{summary}</span>}
          </span>
        }
      >
        <ol
          className="brief-done-changes"
          aria-label={t("brief.done.smallLabel")}
        >
          {groups.map((group) => (
            <GroupStep
              key={group.sentence}
              group={group}
              since={since}
              zone={zone}
              byDay={byDay}
            />
          ))}
        </ol>
      </Disclosure>
    </Card>
  );
}

function GroupStep({
  group,
  since,
  zone,
  byDay,
}: Readonly<{
  group: LineGroup;
  since: string | undefined;
  zone: string;
  byDay: boolean;
}>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const [newest] = group.lines;
  if (!newest) return null;
  // Indigo where a model decided, grey where sync or a rule did: the marks
  // the receipt lines wear.
  const agent = group.lines.some((line) => markKind(line) === "agent");
  return (
    <li className="brief-done-change" data-state={agent ? "open" : "routine"}>
      <p className="brief-done-what">
        {group.sentence}
        {group.records > 1 && (
          <>
            {" "}
            <Badge>
              {plural("brief.done.changeCount", group.records, {
                count: floorFigure(
                  formatNumber(group.records, locale),
                  group.floor,
                  group.records,
                ),
              })}
            </Badge>
          </>
        )}
      </p>
      <time
        className="t-caption brief-done-when"
        dateTime={newest.occurred_at}
        title={formatDateTime(newest.occurred_at, locale, zone)}
      >
        {axisLabel(newest.occurred_at, byDay, locale, zone)}
      </time>
      {group.lines.map((line) => (
        <div className="brief-done-controls" key={line.id}>
          <LineSubject line={line} since={since} />
          <LineUndo line={line} />
          <LineDetail line={line} zone={zone} />
        </div>
      ))}
    </li>
  );
}

/** Who acted, when and why, and why there is no undo where there is none. */
function LineDetail({
  line,
  zone,
}: Readonly<{ line: MagicLine; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const notes = lineNotes(line, t, locale);
  const facts: Fact[] = [
    {
      key: "when",
      term: t("receipt.detail.when"),
      value: formatDateTime(line.occurred_at, locale, zone),
    },
  ];
  if (notes.by) {
    facts.push({ key: "by", term: t("receipt.detail.by"), value: notes.by });
  }
  if (notes.why) {
    facts.push({ key: "why", term: t("brief.done.why"), value: notes.why });
  }
  if (notes.noWayBack) {
    facts.push({
      key: "undo",
      term: t("brief.done.undoNote"),
      value: notes.noWayBack,
    });
  }
  return (
    <Disclosure summary={t("receipt.detail.summary")}>
      <FactList facts={facts} flat />
    </Disclosure>
  );
}

/**
 * The lines in reading order, those that say the same words as one group at
 * the place of its newest line. Grouped by the words a reader sees, so two
 * lines group when they would have read as repeats.
 */
export function groupBySentence(
  rows: readonly MagicLine[],
  sentenceOf: (line: MagicLine) => string,
): LineGroup[] {
  const bySentence = new Map<string, MagicLine[]>();
  for (const line of rows) {
    const sentence = sentenceOf(line);
    const group = bySentence.get(sentence) ?? [];
    group.push(line);
    bySentence.set(sentence, group);
  }
  return [...bySentence].map(([sentence, lines]) => ({
    sentence,
    lines,
    records: lines.reduce((sum, line) => sum + (line.count ?? 1), 0),
    floor: lines.some((line) => line.count_is_floor === true),
  }));
}
