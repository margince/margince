// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";

type JobHealth = components["schemas"]["JobHealth"];

// The maintenance page's one interruption, in its own file because it is the
// one thing on that page that ARGUES rather than reports — and because the
// argument takes two numbers and a rule about which of them is allowed to
// shout.
export function DeadWorkCallout({ health }: Readonly<{ health: JobHealth }>) {
  const t = useT();
  const { locale } = useLocale();
  const dead = health.kinds.reduce((total, kind) => total + kind.dead, 0);
  // TWO NUMBERS, and only one of them interrupts. The week's total is real
  // information — that work did not happen — but River keeps a terminal row for
  // seven days, so an outage that ended an hour ago reads exactly like one still
  // running. What earns the alarm is the recent count; the total goes under it
  // as a figure, so an operator can still see the history without being paged by
  // it.
  const recent = health.kinds.reduce(
    (total, kind) => total + kind.dead_recent,
    0,
  );
  if (recent === 0) {
    return null;
  }
  return (
    // The one thing on this card an operator must not scroll past: dead
    // work does not resume on its own. An `event` would be mentioned
    // quietly; this one interrupts, because the reader has to act on it and
    // nothing else on the page says so again.
    <Callout
      tone="danger"
      kind="event"
      live="alert"
      title={t("jobs.deadTitle", {
        count: formatNumber(recent, locale),
        hours: formatNumber(health.dead_window_hours, locale),
      })}
    >
      <p>{t("jobs.deadBody", { count: formatNumber(recent, locale) })}</p>
      {/* Only when the two differ: "531 recently, 531 this week" is a
              sentence that reads as a second alarm rather than as context. */}
      {dead > recent && (
        <p className="t-sub">
          {t("jobs.deadTotal", { count: formatNumber(dead, locale) })}
        </p>
      )}
    </Callout>
  );
}
