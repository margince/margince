// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { StoryProviders } from "../testing/story-utils";
import type { CreateField } from "./create";
import type { RailEdit } from "./railfield";
import type { FieldRecord } from "./recordfieldtypes";
import { useSectionEdit } from "./sectionedit";

// One details section's Edit: its fields in one dialog, a locked field with its
// reason, and a row that leaves for its own flow.

const meta: Meta = {
  title: "Records/Record 360/Section edit",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const FIELDS: CreateField[] = [
  { key: "title", labelText: "Job title" },
  { key: "close", labelText: "Close date", type: "date" },
  { key: "amount", labelText: "Value", type: "number" },
];

function Section({ canEdit = true }: Readonly<{ canEdit?: boolean }>) {
  return (
    <StoryProviders>
      <AboutSection canEdit={canEdit} />
    </StoryProviders>
  );
}

function AboutSection({ canEdit }: Readonly<{ canEdit: boolean }>) {
  const [record, setRecord] = useState<FieldRecord>({
    id: "d-1",
    version: 1,
    title: "Head of fleet",
    close: "2027-03-31",
    amount: 48000,
  });
  const [flow, setFlow] = useState("");
  const edit: RailEdit = {
    kind: "deal",
    record,
    fields: FIELDS,
    canEdit,
    save: async (values) => {
      setRecord((current) => ({
        ...current,
        ...values,
        version: (current.version ?? 0) + 1,
      }));
    },
  };
  const section = useSectionEdit(edit, {
    label: "About",
    keys: ["title", "close", "amount", "owner"],
    readOnly: {
      amount: { reason: "Set by the signed offer", reading: "€48,000.00" },
    },
    flows: {
      owner: {
        label: "Owner",
        reading: "Ana Brandt",
        open: () => setFlow("The owner handover opened."),
      },
    },
  });
  return (
    <div style={{ maxWidth: 340 }}>
      {section.edit && (
        <Button
          variant="link"
          aria-label="Edit About"
          onClick={section.edit.onEdit}
        >
          Edit
        </Button>
      )}
      <p>{String(record.title ?? "")}</p>
      <p role="status">{flow}</p>
      {section.element}
    </div>
  );
}

async function openSection(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const user = userEvent.setup();
  await user.click(await canvas.findByRole("button", { name: "Edit About" }));
  const dialog = await screen.findByRole("dialog", { name: "About" });
  await waitFor(() => expect(dialog).toBeVisible());
  return { canvas, user, dialog: within(dialog) };
}

export const Dialog: Story = {
  render: () => <Section />,
  play: async ({ canvasElement }) => {
    const { dialog } = await openSection(canvasElement);
    await expect(
      dialog.getByRole("textbox", { name: "Job title" }),
    ).toHaveValue("Head of fleet");
    await expect(
      dialog.getByRole("group", { name: "Value" }),
    ).toHaveAccessibleDescription("Set by the signed offer");
    await expect(
      dialog.getByRole("button", { name: "Change Owner" }),
    ).toBeVisible();
  },
};

export const DialogDark: Story = {
  ...Dialog,
  globals: { theme: "dark" },
};

export const DialogPhone: Story = {
  ...Dialog,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The flow's own row closes the section first, then opens the flow.
export const FlowLeaves: Story = {
  render: () => <Section />,
  play: async ({ canvasElement }) => {
    const { canvas, user, dialog } = await openSection(canvasElement);
    await user.click(dialog.getByRole("button", { name: "Change Owner" }));
    await expect(await canvas.findByRole("status")).toHaveTextContent(
      "The owner handover opened.",
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "About" })).toBeNull(),
    );
  },
};

// A draft is never dropped on the way to a flow without a question.
export const FlowAsksFirst: Story = {
  render: () => <Section />,
  play: async ({ canvasElement }) => {
    const { user, dialog } = await openSection(canvasElement);
    await user.type(dialog.getByRole("textbox", { name: "Job title" }), "s");
    await user.click(dialog.getByRole("button", { name: "Change Owner" }));
    const question = await screen.findByRole("dialog", {
      name: "Discard unsaved changes?",
    });
    await waitFor(() => expect(question).toBeVisible());
  },
};

// A reader who may change only the flow goes straight to it.
export const OnlyAFlow: Story = {
  render: () => <Section canEdit={false} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(await canvas.findByRole("button", { name: "Edit About" }));
    await expect(await canvas.findByRole("status")).toHaveTextContent(
      "The owner handover opened.",
    );
    await expect(screen.queryByRole("dialog")).toBeNull();
  },
};
