// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Compact rows share their title, dates and actions with the full worklist.
// Details expose supporting evidence; sorting diagnostics stay off the agenda.

import type { ReactNode } from "react";
import { Badge } from "../design-system/atoms";
import { Popover } from "../design-system/popover";
import { useTruncationTooltip } from "../design-system/tooltip";
import { useT } from "../i18n";
import { isUnprepared } from "./worklist.copy";
import type { WorklistItem } from "./worklist.queries";
import { VerdictLine } from "./worklist.verdict";
import "./worklist.row.compact.css";

/**
 * Everything the row has already read off its item, in the order a column says
 * it.
 *
 * ONE object, handed to whichever column the surface asked for, so the two
 * densities cannot come to say different things about one piece of work.
 *
 * Strings rather than the item, deliberately: `when` is written against the
 * reader's clock and the record's zone, `facts` against the reader's currency,
 * and the reasons are phrased by `worklist.copy.ts`. A column that took the
 * item and re-derived any of them would be a second author of the row's words.
 */
export type RowReadings = Readonly<{
  item: WorklistItem;
  /** The row's name, and where pressing it goes — no `href`, not a link. */
  title: string;
  href?: string;
  /** The clock this row is racing, and what it is worth. */
  when: string | null;
  facts: string | null;
  /** Why it is here: what the line says outright, and what the fold holds. */
  said: readonly string[];
  folded: readonly string[];
  /** Why it beat the row below it. A full sentence, so never on the line. */
  above: string | null;
  /** What doing nothing costs. */
  consequence: string | null;
  /** The supporting sentence, from every source that sends prose. */
  detail?: string | null;
  /** A grouped row's named members. */
  sample: readonly string[];
  /** The zone a verdict's timestamps are read in. */
  zone: string;
}>;

/**
 * The row's name, and where pressing it goes: the record where the row has an
 * address, the row's own evidence where it has a door instead, and plain text
 * where it has neither.
 */
function RowName({
  title,
  href,
  onOpen,
}: Readonly<{ title: string; href?: string; onOpen?: () => void }>) {
  if (href) {
    return (
      <a className="entity-link" href={href}>
        {title}
      </a>
    );
  }
  if (onOpen) {
    return (
      <button type="button" className="worklist-row-open" onClick={onOpen}>
        {title}
      </button>
    );
  }
  return title;
}

/**
 * A card that has BOTH an address and a pane keeps the pane reachable: the
 * name goes to the record, so the evidence needs a word of its own.
 */
function PaneDoor({
  href,
  onOpen,
}: Readonly<{ href?: string; onOpen?: () => void }>) {
  const t = useT();
  if (!href || !onOpen) {
    return null;
  }
  return (
    <button type="button" className="link-button" onClick={onOpen}>
      {t("brief.focus.context")}
    </button>
  );
}

/**
 * Why the card is here, in one line.
 *
 * The ranking's own comparison first — it names what this row beat. Failing
 * that, what doing nothing costs, but only on a row the day put at its head:
 * "If you do nothing, it slips" under every routine task is a sentence true
 * of all of them, and six copies of it drown the one that names a date.
 */
function cardReason(readings: RowReadings): string | null {
  if (readings.above) {
    return readings.above;
  }
  return readings.item.level <= URGENT_LEVEL ? readings.consequence : null;
}

/** The levels the day treats as urgent: somebody waiting or a promise going. */
const URGENT_LEVEL = 2;

/**
 * What the line folds away: the further reasons, the evidence sentence and a
 * grouped row's named members. Captions in the order a reader would ask for
 * them; the caller decides whether they stand open or behind a press.
 */
function foldedLines(readings: RowReadings): ReactNode[] {
  const { item, folded, detail, sample } = readings;
  const lines: ReactNode[] = [];
  if (folded.length > 0) {
    lines.push(
      <p className="t-caption" key="reasons">
        {folded.join(" · ")}
      </p>,
    );
  }
  if (detail && item.source !== "notice") {
    lines.push(
      <p className="t-caption" key="detail">
        {detail}
      </p>,
    );
  }
  if (sample.length > 0) {
    lines.push(
      <p className="t-caption" key="sample">
        {sample.join(" · ")}
      </p>,
    );
  }
  return lines;
}

/** The linked title opens the record; optional details contain its evidence. */
export function CompactRowLine({
  readings,
  named,
  card = false,
  onOpen,
  hero = false,
}: Readonly<{
  readings: RowReadings;
  /**
   * Whether this line draws the NAME.
   *
   * False where the message above already named the row: a waiting email names
   * itself with the canonical reading of a message (`EmailEntry`, whose whole
   * contract is that it has no compact form), so the line would print the
   * subject a second time. It still carries the day's states and the
   * fragments, which that reading does not answer.
   */
  named: boolean;
  /**
   * The line drawn as a CARD rather than a row: the name on its own line, the
   * facts under it, and the ranking's own reason under those. A card has the
   * height to say why the row is here; a row has to be one line.
   */
  card?: boolean;
  /**
   * The card's own door, where the row has no address of its own — a task
   * whose evidence opens beside the page rather than on a record. The name is
   * the control, because a card whose name is dead text and whose door is a
   * band under it asks the reader to learn two places to press.
   */
  onOpen?: () => void;
  /**
   * The day's LEAD, drawn open: what the fold would hold — the further reasons,
   * the evidence sentence, the named members — stands on the card instead of
   * behind a "Details" press. The first row is the one the reader acts on
   * without opening anything, so it is the one row that may spend the height.
   */
  hero?: boolean;
}>) {
  const { item, title, href, said, zone } = readings;
  const t = useT();
  const asCard = card || hero;
  const reason = asCard ? cardReason(readings) : null;
  // ONE STRING, so there is ONE truncation and one tip over it. Drawn as
  // separate fragments the line would clip whichever happened to be last and
  // leave a reader no way to see what went.
  const inline = [
    readings.when,
    item.move?.action === "draft_reply" ? t("brief.reply.owed") : null,
    readings.facts,
    ...said,
  ]
    .filter((part): part is string => part !== null && part !== "")
    .join(" · ");
  const tip = useTruncationTooltip<HTMLSpanElement>(inline);
  const rest = foldedLines(readings);

  return (
    <div
      className={
        hero ? "worklist-row-line worklist-row-hero" : "worklist-row-line"
      }
    >
      <p className="t-body worklist-row-title">
        {named && <RowName title={title} href={href} onOpen={onOpen} />}
        {/* The day's states ride the name at both densities: a rep scanning for
            the row to open before it starts has to see them without reading
            anything beside them. */}
        {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
        {isUnprepared(item) && (
          <Badge tone="warn">{t("worklist.needsPrep")}</Badge>
        )}
      </p>
      {inline && (
        <span
          className="t-caption worklist-row-inline"
          ref={tip.ref}
          {...tip.trigger}
        >
          {inline}
          {tip.tip}
        </span>
      )}
      {item.source === "notice" && readings.detail && (
        <p className="t-caption worklist-row-notice">{readings.detail}</p>
      )}
      {reason && <p className="t-caption worklist-row-reason">{reason}</p>}
      {asCard && <PaneDoor href={href} onOpen={onOpen} />}
      <VerdictLine verdict={item.verdict} zone={zone} />
      {hero && rest}
      {!hero && rest.length > 0 && (
        <Popover
          // NO `variant`: the catalog's own direction for a trigger that reads
          // as words in the line it sits in rather than as a control — the
          // shape the tags strip's "+N" already wears. A ghost Button here drew
          // a 44px filled chip beside a 19px line.
          className="t-caption worklist-row-why"
          label={t("brief.row.details")}
        >
          {rest}
        </Popover>
      )}
    </div>
  );
}
