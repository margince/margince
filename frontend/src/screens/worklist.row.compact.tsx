// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The row at LIST DENSITY: one line, and everything it cannot carry one press
// away.
//
// Its own file rather than a branch inside `worklist.row.tsx` because what
// differs between the two densities is the TEXT COLUMN's layout and nothing
// else — the rank, the kind, the verbs and the thumb surface around it are the
// row's either way. Every reading this draws arrives already made, so there is
// no second derivation of a title, a clock or a reason here: the row reads the
// item once and both densities print the same answers.
//
// WHY A DENSITY AND NOT A SECOND ROW. The Brief opens with five of these and
// goes on to the rest of the morning; the Worklist is the surface a rep works
// to the end. Same work, same words, same verbs — the reader is scanning in one
// place and reading in the other, and a row that told them different things
// depending on which would be two answers to what a piece of work is.

import type { ReactNode } from "react";
import { Badge } from "../design-system/atoms";
import { Popover } from "../design-system/popover";
import { useTruncationTooltip } from "../design-system/tooltip";
import { formatNumber } from "../format/format";
import { translatePlural, useLocale, useT } from "../i18n";
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
 * The one line, and the press that opens the rest of it.
 *
 * THE TITLE IS THE LINK. The default row draws the way to the record as a verb
 * of its own beside the work; at this density that verb and the title would be
 * two controls on one line reaching the same page, so the name carries it and
 * `RowVerbs` withholds the duplicate (worklist.rowverbs.tsx).
 *
 * NOTHING IS DISCARDED, which is the same shape the default row's fold has: the
 * line says as much as it fits and the count behind it covers everything else
 * the row holds — the reasons that did not fit, why it outranked the row below,
 * the supporting sentence, a group's members, the deal's standing. A count that
 * named only the reasons would promise less than the press delivers.
 */
export function CompactRowLine({
  readings,
  named,
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
}>) {
  const { item, title, href, said, folded, above, zone } = readings;
  const t = useT();
  const { locale } = useLocale();
  // ONE STRING, so there is ONE truncation and one tip over it. Drawn as
  // separate fragments the line would clip whichever happened to be last and
  // leave a reader no way to see what went.
  const inline = [readings.when, readings.facts, ...said, readings.consequence]
    .filter((part): part is string => part !== null && part !== "")
    .join(" · ");
  const tip = useTruncationTooltip<HTMLSpanElement>(inline);
  const rest: ReactNode[] = [];
  if (folded.length > 0) {
    rest.push(
      <p className="t-caption" key="reasons">
        {folded.join(" · ")}
      </p>,
    );
  }
  if (above) {
    rest.push(
      <p className="t-caption" key="above">
        {above}
      </p>,
    );
  }
  if (readings.detail) {
    rest.push(
      <p className="t-caption" key="detail">
        {readings.detail}
      </p>,
    );
  }
  if (readings.sample.length > 0) {
    rest.push(
      <p className="t-caption" key="sample">
        {readings.sample.join(" · ")}
      </p>,
    );
  }
  if (item.verdict) {
    rest.push(<VerdictLine key="verdict" verdict={item.verdict} zone={zone} />);
  }
  return (
    <div className="worklist-row-line">
      <p className="t-body worklist-row-title">
        {!named ? null : href ? (
          <a className="entity-link" href={href}>
            {title}
          </a>
        ) : (
          title
        )}
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
      {rest.length > 0 && (
        // THE POPOVER, not a `<details>`: a fold in a five-row list pushes
        // every row under it down, and the reader comparing two rows loses the
        // second one out from under their eye. Escape closes it and hands the
        // trigger back its focus, both of which the primitive owns.
        //
        // Its own trigger IS the count chip. A `Badge` would be a status pill
        // in one of six SEMANTIC tones, and "+2 more" is not a status; the
        // house already spells this affordance as a small ghost control
        // (`RowTags`, the tags panel), and a badge nested inside a button would
        // be a control named by a decoration.
        <Popover
          // NO `variant`: the catalog's own direction for a trigger that reads
          // as words in the line it sits in rather than as a control — the
          // shape the tags strip's "+N" already wears. A ghost Button here drew
          // a 44px filled chip beside a 19px line.
          className="t-caption worklist-row-why"
          label={
            said.length === 0
              ? // Nothing was said on the line, so there is no "more" to count
                // from and the press names itself — in the words the product
                // already has for this question.
                t("worklist.verdict.rule")
              : translatePlural(locale, "worklist.because.more", rest.length, {
                  count: formatNumber(rest.length, locale),
                })
          }
        >
          {rest}
        </Popover>
      )}
    </div>
  );
}
