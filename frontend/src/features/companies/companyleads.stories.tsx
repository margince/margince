// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyLeads } from "./companyleads";
import { COMPANY_LEADS_VIEW } from "./companyleads.fixtures";

const meta: Meta<typeof CompanyLeads> = {
  title: "Companies/Linked leads",
  component: CompanyLeads,
  args: { companyId: "company-1", view: COMPANY_LEADS_VIEW, loading: false },
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({ lead: ["read"], company: ["read"] }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof CompanyLeads>;
export const Linked: Story = {
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByRole("link", { name: "Dana Buyer" }),
    ).resolves.toBeVisible();
  },
};
export const More: Story = {
  args: {
    view: {
      ...COMPANY_LEADS_VIEW,
      leads: {
        data: COMPANY_LEADS_VIEW.leads?.data ?? [],
        page: { has_more: true, next_cursor: "next-page" },
      },
    },
  },
};
export const Empty: Story = {
  args: {
    view: {
      ...COMPANY_LEADS_VIEW,
      leads: { data: [], page: { has_more: false, next_cursor: null } },
    },
  },
};
export const Withheld: Story = {
  args: {
    view: {
      ...COMPANY_LEADS_VIEW,
      leads: undefined,
      sections_omitted: ["leads"],
    },
  },
};
export const Loading: Story = { args: { view: undefined, loading: true } };
export const Unavailable: Story = {
  args: { view: { ...COMPANY_LEADS_VIEW, leads: undefined } },
};
