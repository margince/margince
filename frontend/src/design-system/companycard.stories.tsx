// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Avatar, Badge, Button, OverflowMenu } from "./atoms";
import { CompanyCard } from "./companycard";
import { CompanyLogo } from "./companylogo";

const meta: Meta<typeof CompanyCard> = {
  title: "Components/Text and data display/Company card",
  component: CompanyCard,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 360 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof CompanyCard>;

const mark = (
  <CompanyLogo
    name="Netcare"
    fallback={<Avatar identity="netcare" name="Netcare" size="sm" />}
  />
);

export const WithFactsAndVerbs: Story = {
  args: {
    logo: mark,
    title: <a href="#/companies/netcare">Netcare</a>,
    badges: <Badge>customer</Badge>,
    facts: "3 contacts · 1 open deal",
    actions: (
      <OverflowMenu label="More actions">
        <Button variant="danger">Detach</Button>
      </OverflowMenu>
    ),
  },
};

export const NameOnly: Story = {
  args: { title: <a href="#/companies/netcare">Netcare</a> },
};
