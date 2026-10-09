// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { RestrictedRecordsCard } from "./restrictedrecords";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Settings → Privacy → Restricted records. Two states worth reviewing: a
// record held under the statutory floor, named by its transactions and its
// deadline; and the empty card, which says every erasure completed in full.

const HELD = {
  activity_id: "00000000-0000-4000-8000-0000000000b1",
  kind: "email",
  occurred_at: "2025-03-04T09:00:00Z",
  restricted_at: "2026-08-18T07:00:00Z",
  restricted_until: "2032-01-01T00:00:00Z",
  reason: "commercial_correspondence · §257 HGB / §147 AO",
  deals: [{ id: "00000000-0000-4000-8000-0000000000d1", name: "Acme rollout" }],
  redacted_fields: ["raw", "counterparty_email"],
};

function restricted(records: unknown[], decide = false, extra: RouteMap = {}) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            allow: {
              retention_policy: decide ? ["read", "update"] : ["read"],
            },
          }),
        ),
      "GET /retention/restrictions": () =>
        jsonResponse({
          data: records,
          page: { next_cursor: null, has_more: false },
        }),
      ...extra,
    });
    return (
      <StoryProviders>
        <RestrictedRecordsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof RestrictedRecordsCard> = {
  title: "Settings/Governance/Privacy and retention/Restricted records",
  component: RestrictedRecordsCard,
};
export default meta;

type Story = StoryObj<typeof RestrictedRecordsCard>;

export const Held: Story = { render: restricted([HELD]) };
export const NothingHeld: Story = { render: restricted([]) };
export const HeldDark: Story = {
  globals: { theme: "dark" },
  render: restricted([HELD]),
};

// The controller's own view: the release action on the row and the pin form
// above it. Both are irreversible and both demand a typed reason, which is
// what the confirm dialog behind them is for.
export const WithTheRetentionAuthority: Story = {
  render: restricted([HELD], true),
};

const HELD_LONG = {
  ...HELD,
  activity_id: "00000000-0000-4000-8000-0000000000b2",
  kind: "meeting",
  deals: [
    {
      id: "00000000-0000-4000-8000-0000000000d2",
      name: "Halloran Seilerei rope refit and five-year maintenance framework",
    },
    { id: "00000000-0000-4000-8000-0000000000d3", name: "Acme renewal" },
  ],
  projects: [
    {
      id: "00000000-0000-4000-8000-0000000000e1",
      name: "Harbour crane survey",
    },
  ],
  redacted_fields: ["raw"],
};

const HELD_PINNED = {
  ...HELD,
  activity_id: "00000000-0000-4000-8000-0000000000b3",
  deals: [],
  redacted_fields: [],
};

// Long deal and project names wrap inside their column; a pinned record names no deal.
export const LongContent: Story = {
  render: restricted([HELD, HELD_LONG, HELD_PINNED], true),
};

// At phone width each row folds: the record and Release, then the rest.
export const HeldPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: restricted([HELD, HELD_LONG, HELD_PINNED], true),
};

export const Loading: Story = {
  render: restricted([], false, {
    "GET /retention/restrictions": () => new Promise<Response>(() => undefined),
  }),
};

export const LoadError: Story = {
  render: restricted([], false, {
    "GET /retention/restrictions": () =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/internal",
          title: "Internal Server Error",
          status: 500,
          code: "internal",
          detail: "The restriction list could not be read.",
        },
        500,
      ),
  }),
};

// A refused release keeps the dialog open over the server's words.
export const ReleaseRefused: Story = {
  render: restricted([HELD], true, {
    [`POST /retention/restrictions/${HELD.activity_id}/release`]: () =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/permission_denied",
          title: "Forbidden",
          status: 403,
          code: "permission_denied",
          detail: "This seat may not release held records.",
        },
        403,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Release email of/i }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.type(dialog.getByRole("textbox"), "Classified wrongly.");
    await userEvent.click(
      dialog.getByRole("button", { name: /release and erase/i }),
    );
    await dialog.findByRole("alert");
  },
};

// Without the retention authority the card keeps its place and says why.
export const Withheld: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow: {} })),
    });
    return (
      <StoryProviders>
        <RestrictedRecordsCard />
      </StoryProviders>
    );
  },
};
