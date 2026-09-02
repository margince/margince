// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ConsentSection } from "./consent";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The Person 360's Art. 7 proof log + DOI redeem field (G-4/G-5). Three
// purposes cover the ternary state matrix in one render: transactional
// (granted, no DOI), events (unknown, no DOI), marketing_email (unknown,
// requiring double opt-in) — the same PURPOSES/CONSENT shapes
// consent.test.tsx exercises, not invented fixtures.

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
    {
      id: "p2",
      key: "events",
      label: "Events",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
    {
      id: "p3",
      key: "marketing_email",
      label: "Marketing",
      requires_double_opt_in: true,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

const CONSENT = {
  state: [
    {
      purpose_id: "p1",
      purpose_key: "transactional",
      state: "granted",
      updated_at: "2026-05-01T10:00:00Z",
    },
    { purpose_id: "p2", purpose_key: "events", state: "unknown" },
    { purpose_id: "p3", purpose_key: "marketing_email", state: "unknown" },
  ],
  events: [
    {
      id: "e1",
      purpose_id: "p1",
      new_state: "granted",
      source: "booking form",
      actor_type: "human",
      actor_id: "u1",
      occurred_at: "2026-05-01T10:00:00Z",
    },
  ],
};

function section(routes: RouteMap) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <ConsentSection personId="person-1" />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConsentSection> = {
  title: "Records/Consent section",
  component: ConsentSection,
};
export default meta;

type Story = StoryObj<typeof ConsentSection>;

// ConsentSection always mounts ConfirmDetailsAction underneath the rows
// (P-8/P-9), and that control's useCanWrite("person", "update") call reaches
// GET /me unconditionally, regardless of which consent state this story is
// about. So every story in this file routes it, granted a full rep seat that
// may write the person, since none of the states below is itself the story
// about the write gate (ConfirmDetails* below are).
const MAY_WRITE = {
  user: { id: "u1", email: "rep@example.test", full_name: "A Rep" },
  authorization: {
    seat_type: "full",
    objects: { person: { read: true, update: true } },
  },
};

export const Default: Story = {
  render: section({
    "GET /consent-purposes": () => jsonResponse(PURPOSES),
    "GET /people/person-1/consent": () => jsonResponse(CONSENT),
    "GET /me": () => jsonResponse(MAY_WRITE),
  }),
};

// G-4: the append-only proof log, toggled open on the already-granted row.
export const ProofLogOpen: Story = {
  render: section({
    "GET /consent-purposes": () => jsonResponse(PURPOSES),
    "GET /people/person-1/consent": () => jsonResponse(CONSENT),
    "GET /me": () => jsonResponse(MAY_WRITE),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const row = (await canvas.findByText("Deal messages")).closest(
      ".consent-row",
    );
    if (!(row instanceof HTMLElement)) {
      throw new Error("consent row not found");
    }
    await userEvent.click(
      within(row).getByRole("button", { name: /proof log/i }),
    );
  },
};

// A workspace that tracks no consent purposes at all — the honest empty
// state, only trusted once the purposes fetch itself has succeeded.
export const Empty: Story = {
  render: section({
    "GET /consent-purposes": () =>
      jsonResponse({ data: [], page: { next_cursor: null, has_more: false } }),
    "GET /people/person-1/consent": () =>
      jsonResponse({ state: [], events: [] }),
    "GET /me": () => jsonResponse(MAY_WRITE),
  }),
};

export const LoadError: Story = {
  render: section({
    "GET /consent-purposes": () => jsonResponse(PURPOSES),
    "GET /people/person-1/consent": () =>
      jsonResponse({ title: "internal error", status: 500 }, 500),
    "GET /me": () => jsonResponse(MAY_WRITE),
  }),
};

// The per-person ask, under the per-purpose rows. It is a MUTATING control, so
// it is drawn only for a caller who may write the person — which is why every
// story below states a seat and a grant rather than leaving /me to the default.
const ROWS: RouteMap = {
  "GET /consent-purposes": () => jsonResponse(PURPOSES),
  "GET /people/person-1/consent": () => jsonResponse(CONSENT),
};

// The link went out. The address is the one the SERVER derived from the
// person's own record — this surface reports it and cannot choose it.
export const ConfirmDetailsSent: Story = {
  render: section({
    ...ROWS,
    "GET /me": () => jsonResponse(MAY_WRITE),
    "POST /people/person-1/consent/confirm-request": () =>
      jsonResponse(
        {
          delivered_to: "ada@example.test",
          expires_at: "2026-09-13T09:00:00Z",
          delivered: true,
        },
        201,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("confirm-details-ask"));
    await canvas.findByTestId("confirm-details-sent");
  },
};

// The token exists and nobody was sent it, because this installation has no
// outbound relay. A rep who read "sent" here would wait for an answer that
// cannot come.
export const ConfirmDetailsUndelivered: Story = {
  render: section({
    ...ROWS,
    "GET /me": () => jsonResponse(MAY_WRITE),
    "POST /people/person-1/consent/confirm-request": () =>
      jsonResponse(
        {
          delivered_to: "ada@example.test",
          expires_at: "2026-09-13T09:00:00Z",
          delivered: false,
        },
        201,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("confirm-details-ask"));
    await canvas.findByTestId("confirm-details-sent");
  },
};

// A contact with no live address. Refused rather than silently not sent, so a
// rep learns there is no mailbox to ask rather than believing they asked.
export const ConfirmDetailsRefused: Story = {
  render: section({
    ...ROWS,
    "GET /me": () => jsonResponse(MAY_WRITE),
    "POST /people/person-1/consent/confirm-request": () =>
      jsonResponse(
        {
          title: "Unprocessable Entity",
          detail:
            "this contact carries no live email address, so there is no mailbox a confirm link could reach",
          status: 422,
        },
        422,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("confirm-details-ask"));
  },
};
