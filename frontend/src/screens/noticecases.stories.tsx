// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
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
// has told them, and by when we must.

type NoticeCase = components["schemas"]["NoticeCase"];

// A fixed 2026 deadline: late for good, so no verdict waits on the clock.
const OVERDUE: NoticeCase = {
  id: "nc-1",
  contact_id: "00000000-0000-4000-8000-0000000000c1",
  contact_name: "Lena Hoffmann",
  acquisition: {
    kind: "purchased_or_imported",
    occurred_at: "2026-01-03T09:00:00Z",
    captured_at: "2026-01-04T09:00:00Z",
    captured_by: "human:u-1",
    captured_by_name: "Anna Weber",
  },
  allowed_routes: ["privacy_notice", "record_confirmation"],
  rule: "art14",
  due_at: "2026-02-03T00:00:00Z",
  state: "open",
  attempts: 0,
  created_at: "2026-01-04T09:00:00Z",
};

const CLAIMED: NoticeCase = {
  id: "nc-2",
  contact_id: "00000000-0000-4000-8000-0000000000c2",
  contact_name: "Jonas Berg",
  acquisition: {
    kind: "mailbox_history",
    occurred_at: null,
    captured_at: "2026-08-30T09:00:00Z",
    captured_by: "connector:gmail",
    captured_by_name: null,
  },
  allowed_routes: ["privacy_notice"],
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
  contact_name: "Mia Klein",
  acquisition: null,
  rule: "art14",
  due_at: "2026-05-01T00:00:00Z",
  state: "provided_elsewhere",
  attempts: 0,
  resolution_note: "Told in the supplier portal when the account was opened",
  created_at: "2026-04-01T09:00:00Z",
};

// A duty whose contact this reader cannot see, resting on no evidence row.
const WITHHELD: NoticeCase = {
  ...OVERDUE,
  id: "nc-4",
  contact_name: null,
  acquisition: null,
};

const LONG: NoticeCase = {
  ...OVERDUE,
  id: "nc-5",
  contact_name: "Maximiliane Theodora von Hohenzollern-Sigmaringen-Wittelsbach",
  acquisition: {
    kind: "requested_quote_or_meeting",
    occurred_at: "2026-01-03T09:00:00Z",
    captured_at: "2026-01-04T09:00:00Z",
    captured_by: "agent:capture",
    captured_by_name: null,
  },
  state: "delivery_failed",
};

// Gated on `privacy_request:read`, worked with `:update`, and a role name
// stands in for neither: meFixture seats an admin holding no object grants.
const WORKS_DUTIES: GrantSpec = {
  privacy_request: ["read", "update"],
  contact: ["update"],
};
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
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: "u-1", display_name: "Anna Weber" }] }),
  };
}

function duties(
  rows: NoticeCase[],
  allow: GrantSpec = WORKS_DUTIES,
  routes: RouteMap = {},
) {
  return () => {
    stubWithSession({ ...queue(rows), ...routes }, allow);
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

// Every row's verbs are drawn whole, inside the card that lists them.
async function verbsFit({ canvasElement }: { canvasElement: HTMLElement }) {
  const canvas = within(canvasElement);
  const verbs = await canvas.findAllByRole("button", { name: /^Actions for/ });
  for (const verb of verbs) {
    const card = verb.closest<HTMLElement>(".panel");
    if (!card) throw new Error("a duty rendered outside its card");
    await expect(verb.scrollWidth).toBeLessThanOrEqual(verb.clientWidth);
    await expect(verb.getBoundingClientRect().right).toBeLessThanOrEqual(
      card.getBoundingClientRect().right,
    );
  }
}

/** An officer's queue: one duty late, one claimed, one ended with a ground. */
export const Owed: Story = {
  render: duties([OVERDUE, CLAIMED, EXCUSED]),
  play: verbsFit,
};

export const OwedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: duties([OVERDUE, CLAIMED, EXCUSED]),
  play: verbsFit,
};

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
      await canvas.findByRole("button", { name: /^Actions for/ }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "End without sending" }),
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

/** A withheld contact and a duty resting on no evidence say so in words. */
export const WithheldAndUnevidenced: Story = {
  render: duties([WITHHELD, EXCUSED]),
};

export const LongContent: Story = { render: duties([LONG, OVERDUE]) };

export const LongContentPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: duties([LONG, OVERDUE]),
};

export const Loading: Story = {
  render: duties([], WORKS_DUTIES, {
    "GET /privacy/notice-cases": () => new Promise(() => {}),
  }),
};

export const ReadFailed: Story = {
  render: duties([], WORKS_DUTIES, {
    "GET /privacy/notice-cases": () =>
      jsonResponse(
        { title: "Internal Server Error", status: 500, code: "internal" },
        500,
      ),
  }),
};

// A refused claim lands on the card, under the queue it was made from.
export const RefusedAssign: Story = {
  render: duties([OVERDUE], WORKS_DUTIES, {
    "POST /privacy/notice-cases/nc-1/assign": () =>
      jsonResponse(
        { title: "Forbidden", status: 403, code: "permission_denied" },
        403,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("combobox", { name: /^Owner of/ }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Anna Weber" }),
    );
    await canvas.findByRole("alert");
  },
};
