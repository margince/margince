// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { isUnprepared } from "./worklist.copy";
import { RowCaptions } from "./worklist.row.captions";
import type { RowReadings } from "./worklist.row.compact";
import { RowName } from "./worklist.rowname";
import { VerdictLine } from "./worklist.verdict";

// Whether the row's name links its record: a message row names itself instead.
export function titleLinks(
  opener: unknown,
  href?: string,
  sourceOpener?: () => void,
): boolean {
  return (
    opener === undefined && sourceOpener === undefined && href !== undefined
  );
}

/**
 * The row's text column at the default density: the name, the prose under it,
 * and the captions under that.
 *
 * Its own component because the row draws one of two columns, with the same
 * readings in both. `WorklistRow` reads the item once and hands the answers to
 * the column the surface asked for. This is the row's own anatomy, and the
 * compact line is its variant.
 */
export function RowText({
  readings,
  emailOpener,
  badged,
}: Readonly<{
  readings: RowReadings;
  /** Opens a waiting message. Its presence is what names the row. */
  emailOpener: ((activityId: string) => void) | undefined;
  /** Whether the day put a state on this row: overdue, or not confirmed ready. */
  badged: boolean;
}>) {
  const {
    item,
    title,
    href,
    when,
    facts,
    said,

    detail,
    sample,
    zone,
  } = readings;
  const t = useT();
  const name = (
    <>
      {emailOpener ? null : (
        <RowName
          title={title}
          href={href}
          open={readings.open}
          unavailable={readings.sourceUnavailable}
        />
      )}
    </>
  );
  const badges = (
    <>
      {item.overdue && <Badge tone="danger">{t("worklist.overdue")}</Badge>}
      {/* A state of the meeting, not a reason among reasons: a rep
              scanning for the one to open before it starts has to see it
              without reading the line under the title. Warn, not danger: an
              unprepared meeting is work to do, not a deadline already missed. */}
      {isUnprepared(item) && (
        <Badge tone="warning">{t("worklist.needsPrep")}</Badge>
      )}
    </>
  );
  return (
    <>
      {(!emailOpener || badged) &&
        (readings.heading ? (
          <div className="worklist-row-title">
            <Heading size="large" as="h3">
              {name}
            </Heading>
            {badges}
          </div>
        ) : (
          <p className="t-body worklist-row-title">
            {name}
            {badges}
          </p>
        ))}
      {readings.supportingTitle && (
        <p className="t-body worklist-row-context">
          {readings.supportingTitle}
        </p>
      )}
      {/* The supporting line, from every source that sends prose: which
          mailbox stopped, why a message bounced or a send was held, what an
          AI task was about, which rule failed and how. On most of these rows
          it is the line that decides. Typed values travel in other fields. */}
      {detail && <p className="t-caption worklist-row-detail">{detail}</p>}
      {sample.length > 0 && (
        // A group nobody can see into is a group nobody trusts, and an
        // untrusted group is worse than the pile it replaced.
        <p className="t-caption worklist-row-sample">{sample.join(" · ")}</p>
      )}
      {/* How the deal is standing, above the captions, not among them. It is
            a reading and they are facts. A reader who cannot tell those apart
            cannot tell what to trust; worklist.verdict.tsx says more. */}
      <VerdictLine verdict={item.verdict} zone={zone} />
      <RowCaptions
        about={readings.about}
        host={readings.host}
        touch={readings.touch}
        when={when}
        facts={facts}
        said={said}
      />
    </>
  );
}

// The rank, with the row's selection checkbox before it where the list offers
// one. Without a checkbox, the lead is the rank alone.
export function RowLead({
  pick,
  children,
}: Readonly<{ pick?: ReactNode; children: ReactNode }>) {
  if (!pick) {
    return <>{children}</>;
  }
  return (
    <span className="worklist-row-lead">
      {pick}
      {children}
    </span>
  );
}

/**
 * The rank, and the way into the pane where there is one.
 *
 * A button on the rank, not a pressable row. The row already holds links and
 * verbs, and a control wrapping controls is a press whose target is a guess.
 *
 * Without `onSelect` it is a plain number. The Brief draws these rows with no
 * second column, so a button there would open nothing. A reader who presses a
 * dead control once learns the page lies about what is pressable.
 */
export function Rank({
  position,
  title,
  selected,
  onSelect,
}: Readonly<{
  position: number;
  title: string;
  selected?: boolean;
  onSelect?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The rank is the page's central claim, so it is readable, not decorative.
  // The list element carries the order for a screen reader; the number states
  // it for everybody else.
  const digit = (
    <span className="worklist-rank">{formatNumber(position, locale)}</span>
  );
  if (!onSelect) {
    return digit;
  }
  return (
    <button
      type="button"
      className="worklist-rank-select"
      aria-pressed={selected ?? false}
      // Names the row, not the verb. One shared label would make the rank
      // buttons alike to anybody navigating by name. It would also override
      // the visible digit, which is what tells them apart on screen.
      aria-label={t("worklist.pane.openRow", {
        position: formatNumber(position, locale),
        title,
      })}
      onClick={onSelect}
    >
      {digit}
    </button>
  );
}
