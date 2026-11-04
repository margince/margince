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

function Rows(props: Readonly<{ onEdit: () => void }>) {
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
        <EditableValue
          editLabel="Change Domain"
          onEdit={onEdit}
          editable={false}
        >
          acme.example
        </EditableValue>
      </FieldRow>
    </FieldGrid>
  );
}

export const Editable: Story = {
  args: { children: "Software" },
  play: async ({ canvasElement, args }) => {
    const user = userEvent.setup();
    const pill = within(canvasElement).getByRole("button", {
      name: "Change Industry Software",
    });
    await user.hover(pill);
    await user.click(pill);
    await expect(args.onEdit).toHaveBeenCalledTimes(1);
  },
};

// Focused, so the fill shows; dark mixes a stronger shade than its row hover.
export const FocusedDark: Story = {
  args: { children: "Software" },
  globals: { theme: "dark" },
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.tab();
    await expect(within(canvasElement).getByRole("button")).toHaveFocus();
  },
};

export const ReadOnly: Story = {
  args: { children: "Software", editable: false },
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
    const user = userEvent.setup();
    await user.click(
      within(canvasElement).getByRole("button", { name: "Change Email" }),
    );
    await expect(args.onEdit).toHaveBeenCalledTimes(1);
  },
};

// A link that does not wrap (an address cut with an ellipsis) stays inside its
// column, and its bubble stays in view.
export const LongLinkValue: Story = {
  args: {
    editLabel: "Change Email",
    link: true,
    children: (
      <a
        className="link-button"
        href="mailto:procurement.department.central@industrial-automation.example"
        style={{
          display: "block",
          minInlineSize: 0,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        procurement.department.central@industrial-automation.example
      </a>
    ),
  },
  decorators: [
    (Story) => (
      <div style={{ inlineSize: "14rem" }}>
        <Story />
      </div>
    ),
  ],
  play: async ({ canvasElement }) => {
    const pill = canvasElement.querySelector(".editable-value");
    const column = pill?.parentElement;
    await expect(pill && column).toBeTruthy();
    await expect(
      (pill?.getBoundingClientRect().right ?? 0) <=
        (column?.getBoundingClientRect().right ?? 0) + 1,
    ).toBe(true);
  },
};

export const InFieldGrid: Story = {
  render: (args) => <Rows onEdit={args.onEdit} />,
};

export const InFieldGridDark: Story = {
  ...InFieldGrid,
  globals: { theme: "dark" },
};

// The grid at phone width. The harness emulates no touch, so the bubble a
// `hover: none` device shows at rest stays hidden here.
export const Phone: Story = {
  ...InFieldGrid,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
