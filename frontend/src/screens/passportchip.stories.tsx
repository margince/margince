// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactElement } from "react";
import { ResolvedPassportChip } from "./passportchip";
import { installFetchStub, StoryProviders } from "./story-utils";

const ROTATED = "01a11ea4-9c2e-7d10-a1b2-3c4d5e6f7a8b";

const meta: Meta<typeof ResolvedPassportChip> = {
  title: "Settings/Governance/Audit log/Passport chip",
  component: ResolvedPassportChip,
};
export default meta;
type Story = StoryObj<typeof ResolvedPassportChip>;

// Both stories stub an empty passport list, since a rotated token has left it.
// Only the row's own answer can then name the client.
function withNoPassports(node: () => ReactElement) {
  return () => {
    installFetchStub({});
    return <StoryProviders>{node()}</StoryProviders>;
  };
}

export const NamedByTheRow: Story = {
  render: withNoPassports(() => (
    <ResolvedPassportChip passportId={ROTATED} agentClient="Claude Desktop" />
  )),
};

export const UnnamedAgent: Story = {
  render: withNoPassports(() => <ResolvedPassportChip passportId={ROTATED} />),
};
