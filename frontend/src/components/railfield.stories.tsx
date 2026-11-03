// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { FieldGrid, FieldRow } from "../design-system/fieldgrid";
import { OffsiteLink } from "../design-system/offsitelink";
import { StoryProviders } from "../testing/story-utils";
import type { CreateField } from "./create";
import { type RailEdit, RailField } from "./railfield";
import type { FieldRecord } from "./recordfieldtypes";

// A record's left column. A plain field or a choice opens a one-field dialog.
// A link keeps its action; its pill opens the edit form.

const meta: Meta = {
  title: "Records/Record 360/Rail field",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const STAGES = [
  { value: "qualify", label: "Qualify" },
  { value: "proposal", label: "Proposal" },
  { value: "won", label: "Won" },
];

const FIELDS: CreateField[] = [
  { key: "name", labelText: "Name", required: true },
  { key: "stage", labelText: "Stage", type: "select", options: STAGES },
  { key: "website", labelText: "Website" },
];

function Rail({ canEdit = true }: Readonly<{ canEdit?: boolean }>) {
  const [record, setRecord] = useState<FieldRecord>({
    id: "d-1",
    version: 1,
    name: "Fleet renewal 2027",
    stage: "proposal",
    website: "https://brandt.example/",
  });
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
  const website = String(record.website ?? "");
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <FieldGrid>
          <FieldRow label="Name">
            <RailField
              edit={edit}
              keys={["name"]}
              label="Name"
              reading={String(record.name ?? "")}
              text={String(record.name ?? "")}
            />
          </FieldRow>
          <FieldRow label="Stage">
            <RailField
              edit={edit}
              keys={["stage"]}
              label="Stage"
              reading={
                STAGES.find((stage) => stage.value === record.stage)?.label
              }
            />
          </FieldRow>
          <FieldRow label="Website">
            <RailField
              edit={edit}
              keys={["website"]}
              label="Website"
              link
              reading={<OffsiteLink href={website}>{website}</OffsiteLink>}
            />
          </FieldRow>
        </FieldGrid>
      </div>
    </StoryProviders>
  );
}

export const Editable: Story = {
  render: () => <Rail />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("button", {
        name: "Change Name Fleet renewal 2027",
      }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("link", { name: /brandt\.example/ }),
    ).toHaveAttribute("href", "https://brandt.example/");
  },
};

export const EditableDark: Story = {
  render: () => <Rail />,
  globals: { theme: "dark" },
};

// A reader who may not edit sees the values plain, the link still a link.
export const ReadOnly: Story = {
  render: () => <Rail canEdit={false} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", { name: /brandt\.example/ }),
    ).toBeVisible();
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};

// A choice opens its dialog; the pick waits for Save.
export const ChoiceDialog: Story = {
  render: () => <Rail />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Change Stage/ }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Stage" });
    await waitFor(() => expect(dialog).toBeVisible());
    await expect(
      within(dialog).getByRole("button", { name: "Save" }),
    ).toBeVisible();
  },
};

// The link's own pill opens the edit form, never the link.
export const LinkFormDialog: Story = {
  render: () => <Rail />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Change Website/ }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Website" });
    await waitFor(() =>
      expect(
        within(dialog).getByDisplayValue("https://brandt.example/"),
      ).toBeVisible(),
    );
  },
};

export const LinkFormDialogPhone: Story = {
  ...LinkFormDialog,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
