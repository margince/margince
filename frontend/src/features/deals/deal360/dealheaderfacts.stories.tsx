// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { FieldGrid } from "../../../design-system/fieldgrid";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealRecordRows } from "./dealheaderfacts";

// The deal's own rows in its details, under the edit form's fields. They are the stage
// it stands on, how it reached Margince and when, and how a closed deal closed.

const meta: Meta = {
  title: "Records/Deal 360/Details rows",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const STAGES = [
  { id: "st-1", name: "Qualified" },
  { id: "st-2", name: "Proposal" },
];

export const OpenDeal: Story = {
  render: () => {
    // The Source row says "entered by you" only of a row the reader captured,
    // so it asks the session who the reader is.
    installFetchStub({ "GET /me": meRoute({ deal: ["read"] }) });
    return (
      <StoryProviders>
        <FieldGrid>
          <DealRecordRows
            deal={{
              stage_id: "st-2",
              source: "manual",
              captured_by: "human:u-1",
              created_at: "2026-06-02T09:00:00Z",
            }}
            stages={STAGES}
            locale="en"
          />
        </FieldGrid>
      </StoryProviders>
    );
  },
};

/** A lost deal says why, and a stage the pipeline cannot name reads as a dash. */
export const LostOnAnArchivedStage: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ deal: ["read"] }) });
    return (
      <StoryProviders>
        <FieldGrid>
          <DealRecordRows
            deal={{
              stage_id: "st-gone",
              status: "lost",
              lost_reason: "Budget frozen until next year",
              source: "csv import",
              captured_by: "human:u-1",
              imported_from: "crm",
            }}
            stages={STAGES}
            locale="en"
          />
        </FieldGrid>
      </StoryProviders>
    );
  },
};

/** Who recorded an imported deal has its own label, apart from its source. */
export const RecordedElsewhereInVietnamese: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ deal: ["read"] }) });
    return (
      <StoryProviders locale="vi">
        <FieldGrid>
          <DealRecordRows
            deal={{
              stage_id: "st-2",
              source: "hubspot",
              captured_by: "human:u-importer",
              author: { display_name: "Mutaz Suleiman", via: "hubspot" },
            }}
            stages={STAGES}
            locale="vi"
          />
        </FieldGrid>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Nguồn ghi nhận")).toBeVisible();
    await expect(
      canvas.getByText("Mutaz Suleiman đã ghi nhận trong HubSpot"),
    ).toBeVisible();
  },
};
