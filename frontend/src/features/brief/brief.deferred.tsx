// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentProps, type ReactNode, Suspense, useState } from "react";
import { PendingBody } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { lazy } from "../../i18n/lazy";
import { APPROVAL_PARAM } from "../../lib/approvalhref";
import { useUrlParams } from "../../lib/urlstate";

// What Home opens on demand stays out of the code and catalog its first paint
// waits for. Each piece suspends inside its own quiet boundary, so opening one
// never swaps the whole screen for the route's pending body.
function Quiet({ children }: Readonly<{ children: ReactNode }>) {
  return <Suspense fallback={null}>{children}</Suspense>;
}

// A piece drawn in the page holds its place while its code arrives.
function Pending({ children }: Readonly<{ children: ReactNode }>) {
  const t = useT();
  return (
    <Suspense fallback={<PendingBody label={t("common.loading")} />}>
      {children}
    </Suspense>
  );
}

const loads = {
  details: () => import("../worklist/taskdetailmodal"),
  pane: () => import("../worklist/worklist.pane"),
  meetingBrief: () => import("./meetingbrief/drawer"),
  worklist: () => import("../worklist/worklist"),
  weeklyPage: () => import("./brief.weeklypage"),
  weekly: () => import("./brief.weekly"),
  approval: () => import("../automations/approvaldrawer"),
};

/** Fetches what Home opens on demand, once its day is on screen. */
export function warmDeferred(): void {
  for (const load of Object.values(loads)) {
    load().catch((error: unknown) =>
      console.warn("Home could not fetch a piece it opens on demand", error),
    );
  }
}

export const TaskDetailModal = lazy(
  async () => ({
    default: (await loads.details()).TaskDetailModal,
  }),
  "complete",
  Quiet,
);

export const MeetingDetailModal = lazy(
  async () => ({
    default: (await loads.details()).MeetingDetailModal,
  }),
  "complete",
  Quiet,
);

export const ActivityDetailModal = lazy(
  async () => ({
    default: (await loads.details()).ActivityDetailModal,
  }),
  "complete",
  Quiet,
);

export const WorklistPane = lazy(
  async () => ({
    default: (await loads.pane()).WorklistPane,
  }),
  "complete",
  Quiet,
);

export const ContactMeetingBrief = lazy(
  async () => ({
    default: (await loads.meetingBrief()).ContactMeetingBrief,
  }),
  "complete",
  Quiet,
);

export const WorklistScreen = lazy<
  NonNullable<
    ComponentProps<typeof import("../worklist/worklist").WorklistScreen>
  >
>(
  async () => ({
    default: (await loads.worklist()).WorklistScreen,
  }),
  "complete",
  Pending,
);

export const WeeklyPage = lazy(
  async () => ({ default: (await loads.weeklyPage()).WeeklyPage }),
  "complete",
  Pending,
);

export const WeekStepper = lazy(
  async () => ({ default: (await loads.weekly()).WeekStepper }),
  "complete",
  Quiet,
);

const ApprovalDrawer = lazy(
  async () => ({
    default: (await loads.approval()).LinkedApprovalDrawer,
  }),
  "complete",
  Quiet,
);

/**
 * A link to one decision, from a notice or a receipt, opens it over Home. Once
 * opened it stays mounted, so closing it plays the drawer's exit.
 */
export function LinkedApprovalDrawer() {
  const [params] = useUrlParams();
  const [asked, setAsked] = useState(false);
  if (params.get(APPROVAL_PARAM) && !asked) setAsked(true);
  return asked ? <ApprovalDrawer /> : null;
}
