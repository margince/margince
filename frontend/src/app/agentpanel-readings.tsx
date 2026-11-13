// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../design-system/applink";
import { formatMoney, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { Spend } from "./agentrail-reads";

/** One figure over its name; a door where the figure has somewhere to go. */
type Reading = Readonly<{
  id: string;
  label: string;
  value: string;
  href?: string;
}>;

/**
 * The agent's figures as one row of instruments: the reader's open tasks,
 * approvals waiting, what settled today and what the month cost. A figure
 * nobody has read, or that this seat may not read, has no cell: a missing
 * reading is not a zero.
 */
export function Readings({
  tasks,
  waiting,
  done,
  spend,
  logHref,
}: Readonly<{
  tasks: number | undefined;
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
    ...count("tasks", t("agent.panel.tasks"), tasks),
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
    <dl className="arreadings" aria-label={t("agent.stat.label")}>
      {readings.map((reading) => (
        <div className="arreading" key={reading.id}>
          <dt className="t-caption">{reading.label}</dt>
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
