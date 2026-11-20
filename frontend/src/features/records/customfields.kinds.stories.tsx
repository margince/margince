// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { en } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { ActivityKindsSelect } from "./customfields.kinds";

// The activity types an activity field shows on. None chosen reads as every
// type, which the placeholder says.
const meta: Meta = {
  title: "Settings/Sales/Records and fields/Activity types",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj;

function Picker({ start }: Readonly<{ start: string[] }>) {
  const [values, setValues] = useState(start);
  return (
    <ActivityKindsSelect
      aria-label={en["cf.activityKinds"]}
      values={values}
      onChange={setValues}
    />
  );
}

export const EveryType: Story = { render: () => <Picker start={[]} /> };

export const SomeTypes: Story = {
  render: () => <Picker start={["call", "meeting"]} />,
};

export const Disabled: Story = {
  render: () => (
    <ActivityKindsSelect
      aria-label={en["cf.activityKinds"]}
      values={["note"]}
      onChange={() => {}}
      disabled
    />
  ),
};
