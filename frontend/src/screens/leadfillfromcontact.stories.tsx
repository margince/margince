// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { LeadScreen } from "./leads";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// "Fill from a contact" under a lead's Details: the way an unnamed lead,
// created against a company, gets its name from a contact the CRM holds.
const meta: Meta = {
  title: "Records/Leads/Fill from a contact",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

export const PickingAContact: Story = {
  render: () => {
    installFetchStub({
      "GET /leads/l-1": () =>
        jsonResponse({
          id: "l-1",
          company_name: "Nordwind Logistik",
          status: "new",
          score: 0,
          source: "manual",
          captured_by: "human:u1",
          writable: true,
          version: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        }),
      "GET /me": meRoute({ lead: ["read", "update"], contact: ["read"] }),
      "GET /contacts": () =>
        jsonResponse({
          data: [
            {
              id: "p-1",
              full_name: "Jonas Petersen",
              primary_email: "jonas@nordwind.example",
              title: "Head of Fleet",
              employer: {
                company_id: "co-1",
                company_name: "Nordwind Logistik",
              },
            },
          ],
          page: { has_more: false, next_cursor: null },
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Fill from a contact" }),
    );
    const panel = within(await within(document.body).findByRole("dialog"));
    await userEvent.type(panel.getByRole("combobox"), "jonas");
    await panel.findByRole("option", { name: /Jonas Petersen/ });
  },
};

/** The same picker over the lead page in dark. */
export const PickingAContactDark: Story = {
  ...PickingAContact,
  globals: { theme: "dark" },
};
