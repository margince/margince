// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import type { GrantSpec } from "../app/mefixture";
import { NoticeCasesCard } from "./noticecases";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// The disclosure-duty queue: who we obtained without asking, whether anybody
// has told them, and by when we must. Four states — a duty already late,
// nothing owed, a reader who may see the queue but not work it, and the seat
// it is withheld from, which says so rather than going absent.

type NoticeCase = components["schemas"]["NoticeCase"];

// A fixed 2026 deadline: late for good, so no verdict waits on the clock.
const OVERDUE: NoticeCase = {
  id: "nc-1",
  contact_id: "00000000-0000-4000-8000-0000000000c1",
  rule: "art14",
  due_at: "2026-02-03T00:00:00Z",
  state: "open",
  attempts: 0,
  created_at: "2026-01-04T09:00:00Z",
};

const CLAIMED: NoticeCase = {
  id: "nc-2",
  contact_id: "00000000-0000-4000-8000-0000000000c2",
  rule: "art13",
  due_at: "2099-11-30T00:00:00Z",
  state: "assigned",
  attempts: 1,
  owner_user_id: "u-1",
  created_at: "2026-08-30T09:00:00Z",
};

const EXCUSED: NoticeCase = {
  id: "nc-3",
  contact_id: "00000000-0000-4000-8000-0000000000c3",
  rule: "art14",
  due_at: "2026-05-01T00:00:00Z",
  state: "provided_elsewhere",
  attempts: 0,
  resolution_note: "Told in the supplier portal when the account was opened",
  created_at: "2026-04-01T09:00:00Z",
};

// Gated on `privacy_request:read`, worked with `:update`, and a role name
// stands in for neither: meFixture seats an admin holding no object grants.
const WORKS_DUTIES: GrantSpec = { privacy_request: ["read", "update"] };
const ONLY_READS: GrantSpec = { privacy_request: ["read"] };

// No zone on the roster entry: the owner picker reads display names and
// nothing else, so naming one would be a decision this surface never makes.
const ROSTER = {
  data: [
    {
      id: "u-1",
      email: "anna@margince.test",
      display_name: "Anna Weber",
      status: "active",
      is_agent: false,
    },
  ],
  page: { next_cursor: null, has_more: false },
};

function queue(rows: NoticeCase[]): RouteMap {
  return {
    "GET /privacy/notice-cases": () =>
      jsonResponse({
        data: rows,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /users": () => jsonResponse(ROSTER),
  };
}

function duties(rows: NoticeCase[], allow: GrantSpec = WORKS_DUTIES) {
  return () => {
    stubWithSession(queue(rows), allow);
    return (
      <StoryProviders>
        <NoticeCasesCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof NoticeCasesCard> = {
  title: "Settings/Governance/Privacy and retention/Notice duties",
  component: NoticeCasesCard,
};
export default meta;

type Story = StoryObj<typeof NoticeCasesCard>;

/** An officer's queue: one duty late, one claimed, one ended with a ground. */
export const Owed: Story = { render: duties([OVERDUE, CLAIMED, EXCUSED]) };

/** Every duty discharged — which is a different sentence from "none found". */
export const NothingOwed: Story = { render: duties([]) };

/** Read without update: the rows are there, the verbs on them are not. */
export const WithoutTheWorkingGrant: Story = {
  render: duties([OVERDUE, CLAIMED], ONLY_READS),
};

/** A seat holding no privacy grant: withheld, and told so rather than shown
 * an empty queue. */
export const Withheld: Story = {
  render: () => {
    stubWithSession(queue([OVERDUE]), {}, { roles: ["rep"], seat: "read" });
    return (
      <StoryProviders>
        <NoticeCasesCard />
      </StoryProviders>
    );
  },
};

// Ending a duty without sending anything: the claim, and the ground the server
// refuses to take it without. ConfirmModal portals, so read off the document.
export const EndingADuty: Story = {
  render: duties([OVERDUE]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "End without sending" }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("Reason, in your own words");
  },
};

// An overdue badge beside a state badge: two tones that must stay tellable
// apart once both inks lift.
export const OwedDark: Story = {
  globals: { theme: "dark" },
  render: duties([OVERDUE, CLAIMED, EXCUSED]),
};
