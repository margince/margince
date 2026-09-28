// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { UTC_ZONE } from "../format/timezone";
import { Field } from "./atoms";
import { TimezoneSelect } from "./timezoneselect";

const meta: Meta<typeof TimezoneSelect> = {
  title: "Design System/Timezone select",
  component: TimezoneSelect,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof meta>;

function Picker({
  open = false,
  initial = UTC_ZONE,
}: Readonly<{ open?: boolean; initial?: string }>) {
  const [zone, setZone] = useState(initial);
  return (
    <Field label="Timezone">
      {(control) => (
        <TimezoneSelect
          {...control}
          value={zone}
          onChange={setZone}
          openOnMount={open}
        />
      )}
    </Field>
  );
}

export const Closed: Story = { render: () => <Picker /> };
export const Open: Story = { render: () => <Picker open /> };
export const Dark: Story = { ...Open, globals: { theme: "dark" } };

export const SavedAlias: Story = {
  render: () => <Picker initial="US/Eastern" />,
};
export const UnknownZone: Story = {
  render: () => <Picker initial="Browser/New_City" />,
};
