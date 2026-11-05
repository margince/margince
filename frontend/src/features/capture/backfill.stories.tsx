// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { BackfillPanel } from "./backfill";

// BackfillPanel stories for the fe-uat render gate, one per branch the panel
// covers. The ordinary setup/running/done/error/cancelled run is seeded via
// `initial`, as the import dialog on a Connections account mounts it off the
// embedded CaptureConnection.backfill. The rest are a provider with no
// Backfiller, a stalled running run, and a refused window narrowing.

type BackfillStatus = components["schemas"]["BackfillStatus"];
type Provider = components["schemas"]["CaptureConnection"]["provider"];

function panelStory(
  provider: Provider,
  initial: BackfillStatus,
  routes: Record<string, (body: unknown) => Response> = {},
) {
  return () => {
    // The poll a live run starts must answer with the same row it was seeded
    // from: left to the stub's fallback it came back as an empty page, and a
    // couple of seconds into the story every live variant collapsed into a
    // finished run with no counts.
    installFetchStub({
      [`GET /connectors/${provider}/backfill`]: () => jsonResponse(initial),
      ...routes,
    });
    return (
      <StoryProviders>
        <BackfillPanel
          provider={provider}
          initial={initial}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof BackfillPanel> = {
  title: "Settings/You/Connections/Backfill",
  component: BackfillPanel,
};
export default meta;
type Story = StoryObj<typeof BackfillPanel>;

export const Setup: Story = {
  render: panelStory(
    "gmail",
    { state: "none" },
    {
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "6m",
          estimated_messages: 1234,
          after_date: "2026-01-23",
          computed_at: "2026-07-23T10:00:00Z",
        }),
    },
  ),
};

// A run that moved a moment ago, so it keeps the progress bar rather than the
// staleness note. Shared by the variants below, which need this same live state.
const RUNNING: BackfillStatus = {
  state: "running",
  backfill_id: "018f3a1b-0000-7000-8000-0000000000b1",
  window: "6m",
  estimated_messages: 400,
  counts: {
    captured: 128,
    contacts_created: 47,
    companies_created: 12,
    messages_scanned: 150,
  },
  updated_at: new Date().toISOString(),
};

const DONE: BackfillStatus = {
  state: "done",
  counts: {
    captured: 512,
    contacts_created: 90,
    companies_created: 20,
    messages_scanned: 600,
  },
};

export const Running: Story = { render: panelStory("gmail", RUNNING) };

export const Done: Story = { render: panelStory("gmail", DONE) };

// The live run in dark: neutral Panel, the grey live "Reading" badge, the Meter.
export const RunningDark: Story = {
  globals: { theme: "dark" },
  render: panelStory("gmail", RUNNING),
};

// The finished run in dark: the same neutral Panel, no tint for "done".
export const DoneDark: Story = {
  globals: { theme: "dark" },
  render: panelStory("gmail", DONE),
};

// The live run at 390px: the StatStrip folds its cards to rows, and the badge
// stays in the panel head.
export const RunningPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: panelStory("gmail", RUNNING),
};

export const ErrorState: Story = {
  render: panelStory("gmail", {
    state: "error",
    counts: { captured: 40, contacts_created: 9 },
    last_error_class: "auth",
  }),
};

export const Cancelled: Story = {
  render: panelStory("gmail", {
    state: "cancelled",
    counts: { captured: 20, messages_scanned: 40 },
  }),
};

// The stopped run, and the way back out of it: pressing "Start a new import"
// puts the window picker in front of the reader again, opened on the window
// that ran. Without it, stopping once left the mailbox with no path to a
// second import at all.
export const RestartAfterCancel: Story = {
  render: panelStory(
    "gmail",
    {
      state: "cancelled",
      window: "12m",
      counts: { captured: 20, messages_scanned: 40 },
    },
    {
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "12m",
          estimated_messages: 890,
          computed_at: "2026-07-23T10:00:00Z",
        }),
    },
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /Start a new import/ }),
    );
    await canvas.findByText("890 emails in that time.");
  },
};

// A running run whose updated_at hasn't moved in 20 minutes: the progress
// bar stops claiming motion and the panel says when it last actually moved.
export const Stale: Story = {
  render: panelStory("gmail", {
    state: "running",
    estimated_messages: 400,
    counts: { captured: 40, messages_scanned: 40 },
    updated_at: new Date(Date.now() - 20 * 60_000).toISOString(),
  }),
};

// No provider-side message count to divide by: absolute counts only, never
// a percentage over a guess.
export const NullEstimate: Story = {
  render: panelStory("gmail", {
    state: "running",
    estimated_messages: null,
    counts: { captured: 12 },
  }),
};

// IMAP has no Backfiller: the setup screen's auto-preview hits
// connector_unsupported. The panel states the capability gap plainly instead of
// rendering a window picker that can only fail again.
export const Unsupported: Story = {
  render: panelStory(
    "imap",
    { state: "none" },
    {
      "POST /connectors/imap/backfill/preview": () =>
        jsonResponse({ code: "connector_unsupported" }, 422),
    },
  ),
};

// A prior wider run already covers more history than the newly-picked window
// would. Start refuses with window_narrowing, and the panel explains the
// widen-only rule rather than a generic failure.
export const Narrowing: Story = {
  render: panelStory(
    "gmail",
    { state: "none" },
    {
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "3m",
          estimated_messages: 200,
          computed_at: "2026-07-23T10:00:00Z",
        }),
      "POST /connectors/gmail/backfill": () =>
        jsonResponse({ code: "window_narrowing" }, 409),
    },
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Start import" }),
    );
    await canvas.findByText(/already went back further/i);
  },
};

export const CappedHistory: Story = {
  render: panelStory(
    "gmail",
    { state: "none" },
    {
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "6m",
          after_date: "2026-01-23",
          computed_at: "2026-07-23T10:00:00Z",
          estimated_messages: 20000,
          estimate_is_floor: true,
          estimated_cost_minor: 300,
          currency: "USD",
        }),
    },
  ),
};

export const CappedHistoryDark: Story = {
  ...CappedHistory,
  globals: { theme: "dark" },
};
