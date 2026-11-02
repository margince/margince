// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { PaletteRow } from "./paletterow";

const meta: Meta<typeof PaletteRow> = {
  title: "Shell/Palette row",
  component: PaletteRow,
  render: (args) => (
    <LocaleProvider initial="en">
      <div className="palette-list">
        <PaletteRow {...args} />
      </div>
    </LocaleProvider>
  ),
  args: {
    previous: undefined,
    selected: false,
    onRun: fn(),
    command: {
      id: "recent:contact",
      label: "Anna Weber",
      subtitle: "Contact",
      type: "record",
      group: "Recent",
      recent: {
        kind: "record",
        entity_type: "contact",
        id: "00000000-0000-4000-8000-000000000002",
        label: "Anna Weber",
      },
      mark: {
        identity: "00000000-0000-4000-8000-000000000002",
        name: "Anna Weber",
      },
    },
  },
};
export default meta;
type Story = StoryObj<typeof PaletteRow>;
export const Recent: Story = {};
export const RecentDark: Story = { globals: { theme: "dark" } };
export const Selected: Story = { args: { selected: true } };
export const SelectedDark: Story = { ...Selected, globals: { theme: "dark" } };
