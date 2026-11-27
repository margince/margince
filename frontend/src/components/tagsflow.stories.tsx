// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { RecordDetails } from "../design-system/recorddetails";
import { en as englishCopy } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { useTagsFlow, useTagsRow } from "./tagsflow";

// A record's Tags row reads its tags; its section's Edit opens their own editor.

const meta: Meta = {
  title: "Records/Record 360/Tags flow",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const tagged = {
  data: [
    {
      tag_id: "t-1",
      name: "Fleet",
      archived: false,
      assigned_at: "2026-01-02T00:00:00Z",
    },
  ],
  withheld: false,
};

function Page({
  grant,
  tags = tagged,
}: Readonly<{ grant: ("read" | "update")[]; tags?: unknown }>) {
  installFetchStub({
    "GET /me": meRoute({ deal: grant, tag: ["read"] }, { seat: "full" }),
    "GET /records/deal/d-1/tags": () => jsonResponse(tags),
  });
  return (
    <StoryProviders>
      <Registration />
    </StoryProviders>
  );
}

function Registration() {
  const record = { id: "d-1", writable: true };
  const rows = useTagsRow("deal", record);
  const tags = useTagsFlow("deal", record);
  return (
    <div style={{ maxWidth: 340 }}>
      <RecordDetails
        sections={[
          {
            key: "registration",
            label: "Registration",
            rows,
            edit: tags.flow && { onEdit: tags.flow.open },
          },
        ]}
      />
      {tags.element}
    </div>
  );
}

export const OpensTheEditor: Story = {
  name: "Edit opens the tag editor",
  render: () => <Page grant={["read", "update"]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Fleet");
    await expect(canvas.queryByRole("button", { name: "Add tag" })).toBeNull();
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit Registration" }),
    );
    const editor = await screen.findByRole("dialog", { name: "Tags" });
    await waitFor(() => expect(editor).toBeVisible());
    await expect(
      within(editor).getByRole("button", { name: "Add tag" }),
    ).toBeVisible();
  },
};

export const OpensTheEditorPhone: Story = {
  ...OpensTheEditor,
  tags: ["uat-phone"],
};

// A reader who may not change the record reads the tags and gets no Edit.
export const Reader: Story = {
  name: "No Edit for a reader",
  render: () => <Page grant={["read"]} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Fleet");
    await expect(canvas.queryByRole("button", { name: /^Edit / })).toBeNull();
  },
};

export const Withheld: Story = {
  name: "Withheld tags say so",
  render: () => <Page grant={["read"]} tags={{ data: [], withheld: true }} />,
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(englishCopy["tags.withheld"]),
    ).toBeVisible();
  },
};
