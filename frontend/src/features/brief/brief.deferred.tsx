// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentProps, type ReactNode, Suspense } from "react";
import { lazy } from "../../i18n/lazy";
import { APPROVAL_PARAM } from "../../lib/approvalhref";
import { useUrlParams } from "../../lib/urlstate";

// What Home opens on demand stays out of the code and catalog its first paint
// waits for. Each piece suspends inside its own quiet boundary, so opening one
// never swaps the whole screen for the route's pending body.
function Quiet({ children }: Readonly<{ children: ReactNode }>) {
  return <Suspense fallback={null}>{children}</Suspense>;
}

export const TaskDetailModal = lazy(
  async () => ({
    default: (await import("../worklist/taskdetailmodal")).TaskDetailModal,
  }),
  "complete",
  Quiet,
);

export const MeetingDetailModal = lazy(
  async () => ({
    default: (await import("../worklist/taskdetailmodal")).MeetingDetailModal,
  }),
  "complete",
  Quiet,
);

export const ActivityDetailModal = lazy(
  async () => ({
    default: (await import("../worklist/taskdetailmodal")).ActivityDetailModal,
  }),
  "complete",
  Quiet,
);

export const WorklistPane = lazy(
  async () => ({
    default: (await import("../worklist/worklist.pane")).WorklistPane,
  }),
  "complete",
  Quiet,
);

export const ContactMeetingBrief = lazy(
  async () => ({
    default: (await import("./meetingbrief/drawer")).ContactMeetingBrief,
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
    default: (await import("../worklist/worklist")).WorklistScreen,
  }),
  "complete",
  Quiet,
);

export const WeeklyPage = lazy(
  async () => ({ default: (await import("./brief.weeklypage")).WeeklyPage }),
  "complete",
  Quiet,
);

export const WeekStepper = lazy(
  async () => ({ default: (await import("./brief.weekly")).WeekStepper }),
  "complete",
  Quiet,
);

const ApprovalDrawer = lazy(
  async () => ({
    default: (await import("../automations/approvaldrawer"))
      .LinkedApprovalDrawer,
  }),
  "complete",
  Quiet,
);

/** A link to one decision, from a notice or a receipt, opens it over Home. */
export function LinkedApprovalDrawer() {
  const [params] = useUrlParams();
  return params.get(APPROVAL_PARAM) ? <ApprovalDrawer /> : null;
}
