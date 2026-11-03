// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { EditableValue } from "./editablevalue";
import { FieldGrid, FieldRow } from "./fieldgrid";

const meta = {
  title: "Components/Forms and input/Editable value",
  component: EditableValue,
  args: { editLabel: "Change Industry", onEdit: fn() },
} satisfies Meta<typeof EditableValue>;
export default meta;

type Story = StoryObj<typeof meta>;

const LONG =
  "Industrial automation, robotics integration and the maintenance contracts that come with a fleet of installed lines across three plants";

function Rows(props: Readonly<{ onEdit?: () => void }>) {
  const { onEdit } = props;
  return (
    <FieldGrid>
      <FieldRow label="Industry">
        <EditableValue editLabel="Change Industry" onEdit={onEdit}>
          Software
        </EditableValue>
      </FieldRow>
      <FieldRow label="Legal name">
        <EditableValue
          editLabel="Change Legal name"
          placeholder="Add legal name"
          onEdit={onEdit}
        />
      </FieldRow>
      <FieldRow label="Email">
        <EditableValue editLabel="Change Email" onEdit={onEdit} link>
          <a className="link-button" href="mailto:ana@example.com">
            ana@example.com
          </a>
        </EditableValue>
      </FieldRow>
      <FieldRow label="Description">
        <EditableValue editLabel="Change Description" onEdit={onEdit}>
          {LONG}
        </EditableValue>
      </FieldRow>
      <FieldRow label="Domain">
        <EditableValue editLabel="Change Domain">acme.example</EditableValue>
      </FieldRow>
    </FieldGrid>
  );
}

export const Editable: Story = {
  args: { children: "Software" },
  play: async ({ canvasElement, args }) => {
    const pill = within(canvasElement).getByRole("button", {
      name: "Change Industry Software",
    });
    await userEvent.hover(pill);
    await userEvent.click(pill);
    await expect(args.onEdit).toHaveBeenCalledTimes(1);
  },
};

// Focused, so the fill shows; dark mixes a stronger shade than its row hover.
export const FocusedDark: Story = {
  args: { children: "Software" },
  globals: { theme: "dark" },
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    await expect(within(canvasElement).getByRole("button")).toHaveFocus();
  },
};

export const ReadOnly: Story = {
  args: { children: "Software", onEdit: undefined },
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByRole("button")).toBeNull();
  },
};

export const Empty: Story = {
  args: { placeholder: "Add industry" },
};

export const LongValue: Story = {
  args: { children: LONG },
};

export const LinkValue: Story = {
  args: {
    editLabel: "Change Email",
    link: true,
    children: (
      <a className="link-button" href="mailto:ana@example.com">
        ana@example.com
      </a>
    ),
  },
  play: async ({ canvasElement, args }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Change Email" }),
    );
    await expect(args.onEdit).toHaveBeenCalledTimes(1);
  },
};

export const InFieldGrid: Story = {
  render: (args) => <Rows onEdit={args.onEdit} />,
};

export const InFieldGridDark: Story = {
  ...InFieldGrid,
  globals: { theme: "dark" },
};

// Storybook cannot emulate `hover: none`; a touch device, or a browser run
// with touch emulation, shows the bubble at rest here.
export const Phone: Story = {
  ...InFieldGrid,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
