// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactElement } from "react";
import { MintedPassport, PassportUses } from "./settings.passportuse";
import { installFetchStub, StoryProviders } from "./story-utils";

const API_BASE = "https://crm.example.com/v1";

const meta: Meta<typeof PassportUses> = {
  title: "Settings/You/Agents/Passport uses",
  component: PassportUses,
};
export default meta;
type Story = StoryObj<typeof PassportUses>;

function withStub(node: () => ReactElement) {
  return () => {
    installFetchStub({});
    return <StoryProviders>{node()}</StoryProviders>;
  };
}

export const Uses: Story = {
  render: withStub(() => <PassportUses apiBaseUrl={API_BASE} />),
};

// Before the passports read answers, the code row has no address to print.
export const UsesWithoutAddress: Story = {
  render: withStub(() => <PassportUses apiBaseUrl={undefined} />),
};

export const Minted: Story = {
  render: withStub(() => (
    <MintedPassport
      token="mgp_7Hq2vXkP9rLw4Tn8sYc1Zb6Ud3Fe0Ga5Jm"
      apiBaseUrl={API_BASE}
    />
  )),
};

export const MintedDark: Story = {
  globals: { theme: "dark" },
  render: Minted.render,
};
