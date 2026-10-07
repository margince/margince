// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Field, SearchField } from "./atoms";

const meta: Meta<typeof SearchField> = {
  title: "Components/Forms and input/Search field",
  component: SearchField,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof SearchField>;

// Only a filled field shows the type="search" clear control.
export const EmptyAndFilled: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-3)", maxWidth: "22rem" }}>
      <Field label="Find a company">
        {(control) => <SearchField {...control} placeholder="Search…" />}
      </Field>
      <Field label="Find a contact">
        {(control) => <SearchField {...control} defaultValue="Anna Brandt" />}
      </Field>
    </div>
  ),
};
