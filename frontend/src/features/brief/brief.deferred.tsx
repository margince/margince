// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, Suspense } from "react";
import { PendingBody } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { lazy } from "../../i18n/lazy";

// What Home opens on demand stays out of the code and catalog its first paint
// waits for. A drawer suspends inside a quiet boundary, so opening one never
// swaps the whole screen for the route's pending body.
export function Quiet({ children }: Readonly<{ children: ReactNode }>) {
  return <Suspense fallback={null}>{children}</Suspense>;
}

// A piece drawn in the page holds its place while its code arrives.
export function Pending({ children }: Readonly<{ children: ReactNode }>) {
  const t = useT();
  return (
    <Suspense fallback={<PendingBody label={t("common.loading")} />}>
      {children}
    </Suspense>
  );
}

const loadMeetingBrief = () => import("./meetingbrief/drawer");
const loadWeeklyPage = () => import("./brief.weeklypage");
const loadWeekly = () => import("./brief.weekly");

let warmed = false;

/**
 * Fetches what Home opens on demand, its own pieces and those named here, once
 * the page is idle. The first click on a task, a meeting or the Worklist then
 * does not wait for its code.
 */
export function warmWhenIdle(loads: readonly (() => Promise<unknown>)[]): void {
  if (warmed) return;
  warmed = true;
  const all = () => {
    for (const load of [
      loadMeetingBrief,
      loadWeeklyPage,
      loadWeekly,
      ...loads,
    ]) {
      load().catch((error: unknown) =>
        console.warn("Home could not fetch a piece it opens on demand", error),
      );
    }
  };
  if (typeof requestIdleCallback === "function")
    requestIdleCallback(all, { timeout: 5_000 });
  else setTimeout(all, 0);
}

export const ContactMeetingBrief = lazy(
  async () => ({ default: (await loadMeetingBrief()).ContactMeetingBrief }),
  "complete",
  Quiet,
);

export const WeeklyPage = lazy(
  async () => ({ default: (await loadWeeklyPage()).WeeklyPage }),
  "complete",
  Pending,
);

export const WeekStepper = lazy(
  async () => ({ default: (await loadWeekly()).WeekStepper }),
  "complete",
  Quiet,
);
