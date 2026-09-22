// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../screens/story-utils";
import { NotificationBell } from "./notificationbell";

// What the strip shows about things waiting, and the panel behind it.
//
// A story here is a set of ANSWERS rather than a set of props: the bell reads
// one endpoint and draws whatever it says, so a fixture is a state a reader is
// genuinely in — a quiet session, a backlog, a centre that has never had
// anything in it.
//
// Mounted in a strip-like row rather than on blank canvas, because the count is
// pinned to the trigger's corner and the panel hangs beneath the bar: floated
// on nothing, both would be reviewed in a geometry that never ships.

type Notice = {
  id: string;
  kind: string;
  subject: string;
  body?: string;
  created_at: string;
  read_at?: string;
  target?: { type: string; id: string };
  origin?: {
    event_id: string;
    actor_type: string;
    actor_id: string;
    occurred_at: string;
  };
};

function notice(
  id: string,
  subject: string,
  extra: Partial<Notice> = {},
): Notice {
  return {
    id,
    kind: "automation_failed",
    subject,
    created_at: "2026-09-15T09:00:00Z",
    ...extra,
  };
}

function story(notices: Notice[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /notices": () =>
        jsonResponse({
          items: notices,
          unread_count: notices.filter((row) => !row.read_at).length,
        }),
    });
    return (
      <StoryProviders>
        <div className="topbar">
          <div className="topbar-lead" />
          <div />
          <div className="topbar-trail">
            <NotificationBell />
          </div>
        </div>
      </StoryProviders>
    );
  };
}

const openIt = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: /notifications/i }),
  );
};

const meta: Meta<typeof NotificationBell> = {
  title: "Shell/Notification bell",
  component: NotificationBell,
  // FULLSCREEN, like the top bar's own stories, and here it decides whether
  // the panel is reviewable at all: the panel is positioned against the
  // viewport the way the app's chrome is, so a canvas that insets the strip
  // leaves the panel where the app would put it and the strip somewhere else —
  // drawn over the bell it hangs from, which is a geometry that never ships.
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof NotificationBell>;

// The resting state: nothing waiting, so the bell carries no mark at all. A
// badge reading "0" is the thing this is a story about NOT doing.
export const Quiet: Story = {
  render: story([
    notice("n1", "An automation could not run", {
      read_at: "2026-09-15T09:30:00Z",
    }),
  ]),
};

// The count on the glyph's corner, which is the only thing most readers ever
// see of this feature.
export const Waiting: Story = {
  render: story([
    notice("n1", "A lead is past its deadline", {
      target: { type: "lead", id: "11111111-1111-4111-8111-111111111111" },
    }),
    notice("n2", "An automation could not run", {
      created_at: "2026-09-14T09:00:00Z",
      body: "The stage-move rule failed on three deals.",
    }),
    notice("n3", "Your mailbox needs signing in to again", {
      created_at: "2026-09-13T09:00:00Z",
    }),
  ]),
};

// The panel open over a mixed history: linked and unlinked subjects, a row an
// agent authored carrying the ONE indigo mark on the surface, and settled rows
// reading as history beneath what is still waiting.
export const CentreOpen: Story = {
  render: story([
    notice("n1", "A draft is ready for you", {
      target: { type: "deal", id: "22222222-2222-4222-8222-222222222222" },
      body: "Written against the last three replies on this deal.",
      origin: {
        event_id: "44444444-4444-4444-8444-444444444444",
        actor_type: "agent",
        actor_id: "agent:drafter",
        occurred_at: "2026-09-15T08:59:00Z",
      },
    }),
    notice("n2", "A capture backlog has stopped moving", {
      created_at: "2026-09-14T09:00:00Z",
    }),
    notice("n3", "An automation could not run", {
      created_at: "2026-09-13T09:00:00Z",
      read_at: "2026-09-13T10:00:00Z",
      target: { type: "company", id: "33333333-3333-4333-8333-333333333333" },
    }),
  ]),
  play: openIt,
};

// A seat nothing has ever told anything. The empty state says what WILL arrive
// here rather than reporting a count of zero.
export const NothingEverRaised: Story = {
  render: story([]),
  play: openIt,
};

// The same panel in dark. The settled rows' ink and the indigo mark are both
// derived tones, so this is where a wrong one shows.
export const CentreOpenDark: Story = {
  globals: { theme: "dark" },
  render: story([
    notice("n1", "A draft is ready for you", {
      origin: {
        event_id: "44444444-4444-4444-8444-444444444444",
        actor_type: "agent",
        actor_id: "agent:drafter",
        occurred_at: "2026-09-15T08:59:00Z",
      },
    }),
    notice("n2", "An automation could not run", {
      created_at: "2026-09-13T09:00:00Z",
      read_at: "2026-09-13T10:00:00Z",
    }),
  ]),
  play: openIt,
};
