// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { CaptureActivityTab } from "./capture-activity";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The states worth seeing side by side are the ones that differ in what the
// surface may HONESTLY say: content it holds, content it deliberately does not,
// and a sender whose question a verdict later closed.

const ENTRIES = [
  {
    id: "01930000-0000-7000-8000-00000000c001",
    connector: "gmail",
    outcome: "captured",
    outcome_now: "captured",
    reason: null,
    activity_id: "01930000-0000-7000-8000-0000000000a1",
    resolution: null,
    counterparty: null,
    subject: null,
    occurred_at: "2026-08-15T09:12:00Z",
  },
  {
    id: "01930000-0000-7000-8000-00000000c002",
    connector: "gmail",
    outcome: "internal",
    outcome_now: "internal",
    reason: "internal_only",
    activity_id: null,
    resolution: null,
    counterparty: null,
    subject: null,
    occurred_at: "2026-08-15T08:41:00Z",
  },
  {
    id: "01930000-0000-7000-8000-00000000c003",
    connector: "telegram",
    outcome: "deferred",
    // Deferred at capture, judged real since — so the server counts it under
    // what the verdict decided rather than under the bucket it was filed in.
    outcome_now: "captured",
    reason: null,
    activity_id: "01930000-0000-7000-8000-0000000000a3",
    resolution: {
      status: "real",
      kind: "person",
      resolved_at: "2026-08-15T09:30:00Z",
    },
    counterparty: null,
    subject: null,
    occurred_at: "2026-08-15T07:55:00Z",
  },
  {
    id: "01930000-0000-7000-8000-00000000c004",
    connector: "gmail",
    outcome: "deferred",
    outcome_now: "deferred",
    reason: "deferral_capped",
    activity_id: "01930000-0000-7000-8000-0000000000a4",
    resolution: null,
    counterparty: null,
    subject: null,
    occurred_at: "2026-08-15T07:30:00Z",
  },
  {
    id: "01930000-0000-7000-8000-00000000c005",
    connector: "imap",
    outcome: "suppressed",
    outcome_now: "suppressed",
    reason: "transactional_registry",
    activity_id: "01930000-0000-7000-8000-0000000000a5",
    resolution: null,
    counterparty: null,
    subject: null,
    occurred_at: "2026-08-15T06:02:00Z",
  },
];

const WINDOW = {
  funnel: { captured: 41, internal: 6, suppressed: 3, deferred: 5, fault: 0 },
  data: ENTRIES,
  page: { next_cursor: null },
  payload_capture_enabled: false,
  window_hours: 24,
  // Five messages still waiting, and the clock they are waiting on. Without it
  // the strip is the state the ticket was about: a number that does not move
  // and no sentence saying when it will.
  sender_verdict: {
    every_seconds: 3600,
    running: false,
    next_pass_at: "2026-08-15T22:21:00Z",
  },
};

function story(body: Record<string, unknown>, allow: GrantSpec = {}) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /capture/activity": () => jsonResponse(body),
      // The block list is composed on this page, so the story owes it a route
      // — the card is what a reader meets first here, and a story that drew it
      // as a failed read would be showing the wrong page.
      "GET /capture/exclusions": () =>
        jsonResponse({
          data: [
            {
              id: "01930000-0000-7000-8000-0000000000e1",
              scope: "user",
              kind: "domain",
              value: "newsletters.example",
              created_at: "2026-08-14T08:00:00Z",
            },
            {
              id: "01930000-0000-7000-8000-0000000000e2",
              scope: "workspace",
              kind: "address",
              value: "billing@vendor.example",
              created_at: "2026-08-10T08:00:00Z",
            },
          ],
        }),
      "GET /capture/activity/workspace": () =>
        jsonResponse({
          ...body,
          data: [ENTRIES[2]],
          funnel: { captured: 4, deferred: 1 },
        }),
    });
    return (
      <StoryProviders>
        <CaptureActivityTab />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CaptureActivityTab> = {
  title: "Settings/You/Capture activity",
  component: CaptureActivityTab,
};
export default meta;
type Story = StoryObj<typeof CaptureActivityTab>;

// The DEFAULT posture, and the one most installations will ever see: the
// pipeline stored no address and no subject, so every row says so rather than
// showing an empty cell a reader would take for a message without a subject.
export const Default: Story = { render: story(WINDOW) };

// With capture.trace_payloads on, an operator is diagnosing. The internal drop
// is the row they turned it on for — it is the only place that content exists.
export const WithPayloadCapture: Story = {
  render: story({
    ...WINDOW,
    payload_capture_enabled: true,
    data: [
      { ...ENTRIES[0], counterparty: "dana@client.io", subject: "Q3 pricing" },
      {
        ...ENTRIES[1],
        counterparty: "colleague@acme.com",
        subject: "Meeting recap",
      },
      // Payload capture is ON and this row still carries nothing: an erased
      // subject. The surface must not present that as the posture being off.
      { ...ENTRIES[3], counterparty: null, subject: null },
    ],
  }),
};

// A manager holds capture_trace, so the shared-channel toggle appears. A rep
// never sees it — hidden rather than disabled, because a control you cannot use
// is an invitation to ask why.
export const WithSharedChannels: Story = {
  render: story(WINDOW, { capture_trace: ["read"] }),
};

export const Empty: Story = {
  render: story({ ...WINDOW, funnel: {}, data: [] }),
};

// Both themes, because every derived value is a color-mix of a canonical token
// and follows the dark accent lift: a surface can be right in light and wrong
// in dark. This story has every tone the page can show at once.
export const WithPayloadCaptureDark: Story = {
  render: story({
    ...WINDOW,
    payload_capture_enabled: true,
    data: [
      { ...ENTRIES[0], counterparty: "dana@client.io", subject: "Q3 pricing" },
      {
        ...ENTRIES[1],
        counterparty: "colleague@acme.com",
        subject: "Meeting recap",
      },
      ENTRIES[3],
    ],
  }),
  globals: { theme: "dark" },
};

// A counter used as a filter. The count line then states BOTH numbers, at the
// head of the log's own row: the strip counts the whole window and the filter
// narrows what has been loaded, so "1" under a counter reading 6 has to say
// which of the two it is.
export const Filtered: Story = {
  render: story(WINDOW),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    // The log is behind a disclosure now, so the story opens it first: the
    // count line this story exists to show sits inside it, and a click with
    // nothing visible behind it demonstrates the opposite of the point.
    await user.click(await canvas.findByText("Messages"));
    await user.click(
      await canvas.findByRole("button", { name: /dropped as internal/i }),
    );
  },
};
