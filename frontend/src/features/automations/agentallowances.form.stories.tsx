// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Panel } from "../../design-system/panel";
import { roleTunedAgentAllowances } from "../../testing/agentallowances.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import {
  draftOf,
  type RoleDrafts,
  roleDraftsOf,
} from "./agentallowances.draft";
import { AllowanceForm } from "./agentallowances.form";

// The allowances card's edit form on its own, with one role value moved so
// the callout says what a save will do.

function Form() {
  const allowances = roleTunedAgentAllowances;
  const [draft, setDraft] = useState(draftOf(allowances));
  const [roleDrafts, setRoleDrafts] = useState<RoleDrafts>({
    ...roleDraftsOf(allowances.roles),
    rep: {
      agent_writes_per_window: "",
      agent_egress_per_window: "",
      agent_calls_per_window: "1500",
    },
  });
  return (
    <AllowanceForm
      allowances={allowances}
      locks={[]}
      draft={draft}
      roleDrafts={roleDrafts}
      toDefaults={false}
      locale="en"
      busy={false}
      onDraft={setDraft}
      onRoleDrafts={setRoleDrafts}
      onToDefaults={() => undefined}
      onSave={() => undefined}
      onCancel={() => undefined}
    />
  );
}

const meta: Meta<typeof AllowanceForm> = {
  title: "Settings/AI/AI usage/Agent allowances form",
  component: AllowanceForm,
};
export default meta;
type Story = StoryObj<typeof AllowanceForm>;

export const ARoleValueMoved: Story = {
  render: () => (
    <StoryProviders>
      <Panel title="Agent passport allowances">
        <Form />
      </Panel>
    </StoryProviders>
  ),
};
