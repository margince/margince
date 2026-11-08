// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Panel } from "../../design-system/panel";
import { roleTunedAgentAllowances } from "../../testing/agentallowances.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { roleDraftsOf } from "./agentallowances.draft";
import { RolesForm, RolesReadout } from "./agentallowances.roles";

// The By role part of the allowances card on its own. At rest it shows each
// role's values and sources; editing shows the boxes an admin types into.

function Editing() {
  const [drafts, setDrafts] = useState(
    roleDraftsOf(roleTunedAgentAllowances.roles),
  );
  return (
    <RolesForm
      allowances={roleTunedAgentAllowances}
      drafts={drafts}
      busy={false}
      onDrafts={setDrafts}
    />
  );
}

const meta: Meta<typeof RolesReadout> = {
  title: "Settings/AI/AI usage/Agent allowances by role",
  component: RolesReadout,
};
export default meta;
type Story = StoryObj<typeof RolesReadout>;

export const AtRest: Story = {
  render: () => (
    <StoryProviders>
      <Panel title="Agent passport allowances">
        <RolesReadout allowances={roleTunedAgentAllowances} />
      </Panel>
    </StoryProviders>
  ),
};

export const RoleBoxes: Story = {
  render: () => (
    <StoryProviders>
      <Panel title="Agent passport allowances">
        <Editing />
      </Panel>
    </StoryProviders>
  ),
};
