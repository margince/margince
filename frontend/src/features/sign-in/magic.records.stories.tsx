// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { MagicLine } from "./magic.queries";
import { LineRecordsOpener } from "./magic.records";

type MagicLineRecords = components["schemas"]["MagicLineRecords"];

const LINE: MagicLine = {
  id: "00000000-0000-7000-8000-000000000001",
  occurred_at: "2026-09-13T07:30:00Z",
  lane: "done",
  summary: {
    key: "magic.action.fields_changed",
    values: { fields: "industry" },
  },
  entity: {
    type: "company",
    id: "00000000-0000-7000-8000-0000000000c1",
    label: "GEM",
  },
  actor: { type: "agent", id: "runner" },
  count: 2,
};

const RECORDS: MagicLineRecords = {
  data: [
    {
      audit_id: "00000000-0000-7000-8000-0000000000a1",
      occurred_at: "2026-09-13T07:30:00Z",
      entity: {
        type: "company",
        id: "00000000-0000-7000-8000-0000000000c1",
        label: "GEM",
      },
      changes: [
        { field: "industry", after: "Software" },
        {
          field: "offer_summary",
          before: "Inventory planning",
          after:
            "Inventory planning and warehouse scheduling. Teams compare supplier lead times, reserve stock, and review shortages across locations. The delivery review includes a documented exception queue and ownership for every open shortage.",
        },
      ],
      undo: {
        undoable: true,
        audit_id: "00000000-0000-7000-8000-0000000000a1",
        version: 3,
      },
    },
    {
      audit_id: "00000000-0000-7000-8000-0000000000a2",
      occurred_at: "2026-09-13T07:29:00Z",
      entity: {
        type: "company",
        id: "00000000-0000-7000-8000-0000000000c2",
        label: "Kandu GmbH",
      },
      changes: [{ field: "industry", before: "Retail", after: "Software" }],
      undo: { undoable: false, reason: "already_undone" },
    },
  ],
  page: { has_more: false, total: 2 },
};

function opener(args: ComponentProps<typeof LineRecordsOpener>) {
  installFetchStub({
    [`GET /magic/lines/${LINE.id}/records`]: () => jsonResponse(RECORDS),
  });
  return (
    <StoryProviders>
      <LineRecordsOpener {...args} />
    </StoryProviders>
  );
}

const meta = {
  title: "Shell/Home/Receipt line records",
  component: LineRecordsOpener,
  args: {
    line: LINE,
    since: "2026-09-12T08:00:00Z",
    // A line names its records, and the names are what opens them.
    summary: "GEM and 1 more",
  },
  render: opener,
} satisfies Meta<typeof LineRecordsOpener>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Opened: Story = {
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "GEM and 1 more" }),
    );
    const dialog = within(await body.findByRole("dialog"));
    await dialog.findByRole("heading", { name: en["magic.records.title"] });
    await dialog.findByText("Kandu GmbH");
  },
};

export const ArchivedMeeting: Story = {
  render: (args) => {
    installFetchStub({
      [`GET /magic/lines/${LINE.id}/records`]: () =>
        jsonResponse({
          data: [
            {
              audit_id: "00000000-0000-7000-8000-0000000000a3",
              occurred_at: "2026-09-13T07:30:00Z",
              entity: {
                type: "activity",
                id: "00000000-0000-7000-8000-0000000000b1",
                label: "Planning review",
              },
              changes: [
                { field: "archived_at", after: "2026-09-13T07:30:00Z" },
              ],
              undo: {
                undoable: true,
                audit_id: "00000000-0000-7000-8000-0000000000a3",
                version: 4,
              },
            },
          ],
          page: { has_more: false, total: 1 },
        }),
    });
    return (
      <StoryProviders>
        <LineRecordsOpener {...args} summary="Planning review" />
      </StoryProviders>
    );
  },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Planning review" }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByRole("button", { name: "Planning review" });
    await dialog.findByRole("button", { name: "Undo" });
  },
};
