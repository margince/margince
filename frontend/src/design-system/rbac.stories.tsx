// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { LocaleProvider } from "../i18n";
import { FieldGuard, RoleBadge } from "./rbac";

const meta: Meta<typeof RoleBadge> = {
  title: "Design System/Role badge and field guard",
  component: RoleBadge,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof RoleBadge>;

const ROW: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: "var(--space-2)",
  alignItems: "center",
};

/**
 * The six seeded system roles. Each is a translated name, so this is the row a
 * reader compares when they are deciding which of two people can do a thing.
 */
export const SeededRoles: Story = {
  render: () => (
    <span style={ROW}>
      {["admin", "management", "manager", "rep", "read_only", "ops"].map(
        (role) => (
          <RoleBadge key={role} roleKey={role} />
        ),
      )}
    </span>
  ),
};

/**
 * A workspace-defined role outside the seeded set renders as its RAW KEY rather
 * than as an invented label. A prettified guess would read as a name somebody
 * chose, and this component has no way to know what the installation meant.
 */
export const WorkspaceDefinedRole: Story = {
  render: () => (
    <span style={ROW}>
      <RoleBadge roleKey="admin" />
      <RoleBadge roleKey="regional_lead" />
    </span>
  ),
};

/**
 * `FieldGuard` is the difference between "withheld" and "absent", which is a
 * distinction a blank space cannot make: omitting the node reads as there being
 * no data, and a reader then draws a conclusion about the record rather than
 * about their own permissions. The masked arm carries `role="img"` with a
 * spoken name, so a screen reader hears the withholding too.
 */
export const VisibleAndMasked: Story = {
  render: () => (
    <span style={ROW}>
      <FieldGuard mode="visible">+49 30 901820</FieldGuard>
      <FieldGuard mode="masked" />
    </span>
  ),
};
