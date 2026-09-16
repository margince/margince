// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CaptureActivityDrawer } from "./capture-activity-drawer";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";
import "./capture-activity.css";

// One message's journey, drilled into from the row it was opened on.
//
// The states here are the different CLAIMS this surface can make: the ladder
// as the server sent it, a message this reader may not open, a deployment
// holding no payloads, and a read that did not land — drawn as "unavailable"
// rather than "none", because every registered stage has a rung and nothing at
// all means the read failed.

type Rung = components["schemas"]["PipelineStageRung"];
type TraceEntry = components["schemas"]["CaptureTraceEntry"];

const TRACE_ID = "01930000-0000-7000-8000-00000000c003";
const TRACE_ROUTE = `GET /capture/traces/${TRACE_ID}`;

function rung(
  over: Partial<Rung> & Pick<Rung, "stage" | "order" | "status">,
): Rung {
  return { subject_kind: "message", ...over };
}

const STAGES: Rung[] = [
  rung({ stage: "connector_filter", order: 10, status: "done" }),
  rung({ stage: "ingress_gate", order: 20, status: "done" }),
  rung({ stage: "erasure_check", order: 30, status: "not_applicable" }),
  rung({ stage: "internal_drop", order: 40, status: "skipped" }),
  rung({ stage: "activity_write", order: 50, status: "done" }),
  rung({
    stage: "tier_ladder",
    order: 60,
    status: "done",
    subject_kind: "sender",
  }),
  rung({
    stage: "verdict",
    order: 80,
    status: "pending",
    subject_kind: "sender",
    reason: "awaiting_pass",
  }),
  rung({
    stage: "company_triage",
    order: 90,
    status: "unknown",
    subject_kind: "domain",
  }),
];

// The row the drawer was opened from. It is what knows WHICH message the
// ladder is about — the trace read is keyed by the trace and answers rungs.
const MESSAGE: TraceEntry = {
  id: TRACE_ID,
  connector: "telegram",
  outcome: "deferred",
  outcome_now: "captured",
  activity_id: "01930000-0000-7000-8000-0000000000a3",
  subject: "Re: rollout dates",
  occurred_at: "2026-08-15T07:55:00Z",
};

const PROVIDERS = {
  data: [
    {
      provider: "telegram",
      label: "Telegram",
      credential_model: "workspace_bot",
      supplies_transport: true,
    },
  ],
};

function drawer(routes: RouteMap, message?: TraceEntry) {
  return () => {
    stubWithSession(
      { "GET /channel-providers": () => jsonResponse(PROVIDERS), ...routes },
      { capture_trace: ["read"] },
    );
    return (
      <StoryProviders>
        <CaptureActivityDrawer
          traceId={TRACE_ID}
          message={message}
          onOpenEmail={() => {}}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CaptureActivityDrawer> = {
  title: "Settings/You/Capture activity/Pipeline drawer",
  component: CaptureActivityDrawer,
};
export default meta;

type Story = StoryObj<typeof CaptureActivityDrawer>;

const landed = () =>
  jsonResponse({
    activity_id: MESSAGE.activity_id,
    connector: "telegram",
    payload_capture_enabled: true,
    retention_hours: 720,
    stages: STAGES,
  });

// The ladder as a reader meets it: the transport named in words, the message
// cited the way every citation of a message in the product is, and a rung per
// registered stage.
export const Ladder: Story = {
  render: drawer({ [TRACE_ROUTE]: landed }, MESSAGE),
};

// Opened from a row whose message moved out of this reader's scope: no
// `activity_id`, so the citation names the message and offers nothing to
// press rather than handing back a link that proves the row exists.
export const MessageOutOfScope: Story = {
  render: drawer({ [TRACE_ROUTE]: landed }, { ...MESSAGE, activity_id: null }),
};

// The posture is off, so no rung carries a sender or a subject. The ladder
// says that once, at the foot — which is a different statement from a rung
// that simply has none.
export const PayloadCaptureOff: Story = {
  render: drawer(
    {
      [TRACE_ROUTE]: () =>
        jsonResponse({
          connector: "telegram",
          payload_capture_enabled: false,
          retention_hours: 720,
          stages: STAGES,
        }),
    },
    MESSAGE,
  ),
};

// The read did not land. `unavailable`, never `empty`: a ladder always has a
// rung per stage, so nothing here means the read failed, and drawing it as
// "there are none" would state a fact about the pipeline we do not have.
export const ReadUnavailable: Story = {
  render: drawer(
    {
      [TRACE_ROUTE]: () =>
        jsonResponse({ title: "Unavailable", status: 503 }, 503),
    },
    MESSAGE,
  ),
};
