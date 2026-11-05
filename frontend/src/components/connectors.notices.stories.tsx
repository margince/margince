// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { StoryProviders } from "../testing/story-utils";
import { OAuthOutcomeNote } from "./connectors.notices";

// What comes back from a consent round trip. The note takes the landing as
// props, so each frame poses the outcome, its provider and the roster it is
// read against.
//
// `ok` names the vendor and what now syncs. A calendar refused after its
// mailbox is a partial success with one way on. The other failures keep one
// heading, since nothing was connected, and their own remedy under it.

type CaptureConnection = components["schemas"]["CaptureConnection"];

const meta: Meta<typeof OAuthOutcomeNote> = {
  title: "Settings/You/Connections/OAuth return",
  component: OAuthOutcomeNote,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof OAuthOutcomeNote>;

const doneBackfill: CaptureConnection["backfill"] = { state: "done" };

function connection(
  provider: CaptureConnection["provider"],
  backfill: CaptureConnection["backfill"] = doneBackfill,
): CaptureConnection {
  return {
    id: `connection-${provider}`,
    provider,
    can_backfill: true,
    status: "connected",
    scopes: [],
    backfill,
  };
}

const offerCalendar = {
  onConnect: () => undefined,
  pending: false,
  error: null,
};

function landing(
  outcome: string,
  provider: string | undefined,
  connections: CaptureConnection[] = [],
) {
  return () => (
    <StoryProviders>
      <OAuthOutcomeNote
        outcome={outcome}
        provider={provider}
        connections={connections}
        connectCalendar={offerCalendar}
      />
    </StoryProviders>
  );
}

const googleImporting = [
  connection("gmail", { state: "running" }),
  connection("gcal"),
];

/** Mail and calendar granted in one trip, and the import of past email running. */
export const Connected: Story = {
  render: landing("ok", "gcal", googleImporting),
};

/** The same landing in dark. */
export const ConnectedDark: Story = {
  globals: { theme: "dark" },
  render: landing("ok", "gcal", googleImporting),
};

/** Only the mailbox was asked for. */
export const MailOnly: Story = {
  render: landing("ok", "graph", [connection("graph")]),
};

/** Only the calendar, as Meetings asks for it: no mailbox is claimed. */
export const CalendarOnly: Story = {
  render: landing("ok", "gcal"),
};

/** The mailbox held and the calendar was declined: one button asks again. */
export const CalendarDeclined: Story = {
  render: landing("denied", "gcal", [connection("gmail")]),
};

/** The same partial success in dark, where the warning reaches the heading's ink. */
export const CalendarDeclinedDark: Story = {
  globals: { theme: "dark" },
  render: landing("denied", "gcal", [connection("gmail")]),
};

/** The reader said no on the first consent. Nothing is wrong and nothing was connected. */
export const Declined: Story = { render: landing("denied", "gmail") };

/** The provider's API was never enabled: an administrator's console, not a retry. */
export const Misconfigured: Story = {
  render: landing("misconfigured", "gmail"),
};

/** This deployment's client credentials were refused: the app card, not a retry. */
export const BadClient: Story = { render: landing("bad_client", "graph") };

/** The same refusal in dark, where tone reaches the heading's ink. */
export const BadClientDark: Story = {
  globals: { theme: "dark" },
  render: landing("bad_client", "graph"),
};

/**
 * A landing carrying no known outcome draws nothing. A frame can only show
 * that; connectors.notices.test.tsx asserts the inherited-member guard.
 */
export const NoOutcome: Story = { render: landing("unrecognised", "gmail") };
