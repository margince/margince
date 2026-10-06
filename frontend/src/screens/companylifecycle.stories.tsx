// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { CompanyLifecycleControl } from "./companylifecycle";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The account's stage beside its name: the design system's dropdown worn as a
// filled button at the control height. The header stories show it in place.
const meta: Meta = {
  title: "Records/Company 360/Lifecycle",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Company = components["schemas"]["Company"];

const company: Company = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-1",
  writable: true,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  version: 1,
};

function Control({ record = company }: Readonly<{ record?: Company }>) {
  installFetchStub({ "GET /me": meRoute({ company: ["read", "update"] }) });
  return (
    <StoryProviders>
      <CompanyLifecycleControl company={record} />
    </StoryProviders>
  );
}

export const Open: Story = {
  render: () => <Control />,
  play: async () => {
    await userEvent.click(
      await screen.findByRole("combobox", { name: "Lifecycle" }),
    );
  },
};

export const OpenDark: Story = { ...Open, globals: { theme: "dark" } };

// Somebody else's account: the stage, with no control that would refuse.
export const ReadOnly: Story = {
  render: () => <Control record={{ ...company, writable: false }} />,
};
