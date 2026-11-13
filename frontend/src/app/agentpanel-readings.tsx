// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check } from "lucide-react";
import { AppLink } from "../design-system/applink";
import { tallyDashes } from "../design-system/margince-core";
import { formatMoney, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { Spend } from "./agentrail-reads";

/**
 * The reader's open tasks as the orb draws them: one segment each around the
 * count. A day read empty closes the ring around a tick.
 */
export function TaskDial({ count }: Readonly<{ count: number }>) {
  const { locale } = useLocale();
  return (
    <span className="ardial" aria-hidden="true">
      <svg viewBox="0 0 100 100" aria-hidden="true">
        <circle className="ardialtrack" cx="50" cy="50" r="46" />
        {tallyDashes(count).map((mark, index) => (
          <circle
            // biome-ignore lint/suspicious/noArrayIndexKey: the segments are interchangeable marks, counted rather than named
            key={index}
            className="ardialmark"
            cx="50"
            cy="50"
            r="46"
            pathLength="100"
            strokeDasharray={mark.dash}
            strokeDashoffset={mark.offset}
          />
        ))}
      </svg>
      <span className="ardialcount t-num">
        {count === 0 ? <Check /> : formatNumber(count, locale)}
      </span>
    </span>
  );
}

/** One figure beside its name; a door where the figure has somewhere to go. */
type Reading = Readonly<{
  id: string;
  label: string;
  value: string;
  href?: string;
}>;

/**
 * The agent's figures on one line: approvals waiting, what settled today and
 * what the month cost. A figure nobody has read, or that this seat may not
 * read, is left out: a missing reading is not a zero.
 */
export function Readings({
  waiting,
  done,
  spend,
  logHref,
}: Readonly<{
  waiting: number | undefined;
  done: number | undefined;
  spend: Spend;
  /** Where the month's cost is itemised, for a seat that may open it. */
  logHref: string | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const count = (
    id: string,
    label: string,
    figure: number | undefined,
    href?: string,
  ): readonly Reading[] =>
    figure === undefined
      ? []
      : [{ id, label, value: formatNumber(figure, locale), href }];
  const readings: readonly Reading[] = [
    ...count(
      "approvals",
      t("agent.panel.decisions"),
      waiting,
      // A real zero is an answer, not a door: nobody has to act on it.
      waiting ? "#/worklist" : undefined,
    ),
    ...count("done", t("agent.stat.done"), done),
    ...(spend.allowed && spend.minor !== undefined
      ? [
          {
            id: "cost",
            label: t("agent.stat.cost"),
            value: formatMoney(spend.minor, spend.currency, locale),
            href: logHref,
          },
        ]
      : []),
  ];
  if (readings.length === 0) return null;
  return (
    <dl className="arreadings t-caption">
      {readings.map((reading) => (
        <div className="arreading" key={reading.id}>
          <dt>{reading.label}</dt>
          <dd className="arfigure t-num">
            {reading.href ? (
              <AppLink
                className="arfiguredoor"
                href={reading.href}
                aria-label={`${reading.label} ${reading.value}`}
              >
                {reading.value}
              </AppLink>
            ) : (
              reading.value
            )}
          </dd>
        </div>
      ))}
    </dl>
  );
}
