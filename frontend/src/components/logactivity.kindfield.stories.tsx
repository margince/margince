// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { ActivityDraft } from "../lib/activitybody";
import { ActivityKindField } from "./logactivity.kindfield";

const meta: Meta<typeof ActivityKindField> = {
  title: "Patterns/Activity type",
  component: ActivityKindField,
};
export default meta;
type Story = StoryObj<typeof ActivityKindField>;
function Kinds() {
  const [value, onChange] = useState<ActivityDraft["kind"]>("message");
  return (
    <ActivityKindField
      value={value}
      offersTask
      offersMessage
      onChange={onChange}
    />
  );
}
export const WithMessage: Story = { render: () => <Kinds /> };
export const WithMessageDark: Story = {
  ...WithMessage,
  globals: { theme: "dark" },
};
