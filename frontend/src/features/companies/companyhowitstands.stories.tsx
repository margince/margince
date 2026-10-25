// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { company, company360 } from "../../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyHowItStands } from "./companyhowitstands";

const meta = {
  title: "Records/Company 360/How it stands",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;
type Company360 = components["schemas"]["Company360"];
type Activity = components["schemas"]["Activity"];

const written = {
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-08-13T15:00:00Z",
  updated_at: "2026-08-13T15:00:00Z",
  is_done: true,
} as const;

const wrote: Activity = {
  ...written,
  id: "a-wrote",
  kind: "email",
  direction: "inbound",
  subject: "Re: retrofit timeline",
  occurred_at: "2026-08-01T08:00:00Z",
  occurred_precision: "instant",
};

function view(activities: Activity[]): Company360 {
  return {
    ...company360,
    as_of: "2026-08-13T09:00:00Z",
    health: {
      relationship: {
        rating: "good",
        reason: "Last met them 24 days ago",
        reason_code: "last_met",
        reason_params: { days: 24 },
      },
    },
    activities: {
      data: activities,
      page: { has_more: false, next_cursor: null },
    },
  };
}

function render(read: Company360) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ company: ["read"] }),
      [`GET /companies/${company.id}/finance-summary`]: () =>
        jsonResponse({
          company_id: company.id,
          state: "connected",
          net_invoiced: { amount_minor: 6_420_000, currency: "EUR" },
          overdue: { amount_minor: 0, currency: "EUR" },
          median_days_after_due: 1,
          payment_behaviour: [0, 2, 1, 0],
        }),
    });
    return (
      <StoryProviders>
        <CompanyHowItStands
          companyId={company.id}
          view={read}
          loading={false}
          failed={false}
          onOpenTab={() => undefined}
        />
      </StoryProviders>
    );
  };
}

export const Standing: Story = { render: render(view([wrote])) };

// A visit logged by its day alone, on the day the page was read. It has
// begun by the morning read, and its mark names the day without a clock.
export const DayOnlyVisit: Story = {
  render: render(
    view([
      wrote,
      {
        ...written,
        id: "a-visit",
        kind: "note",
        subject: "Site visit",
        occurred_at: "2026-08-13T12:00:00Z",
        occurred_precision: "day",
      },
    ]),
  ),
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: en["timeline.kind.note"] }),
    );
    const row = (await screen.findByText("Site visit")).parentElement;
    await expect(row?.textContent).toMatch(/13 Aug/);
    await expect(row?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};
