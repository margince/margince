// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { Receipt } from "../lib/handled";
import { OpenSourceProvider } from "../lib/opensource";
import { StoryProviders } from "../testing/story-utils";
import { automaticDateReceipt } from "../testing/worklist.receiptreview.fixtures";
import { ReceiptDetail } from "./receiptdetail";

const meta: Meta<typeof ReceiptDetail> = {
  title: "Components/AI and provenance/Receipt detail",
  component: ReceiptDetail,
};
export default meta;
type Story = StoryObj<typeof ReceiptDetail>;

const receipt: Receipt = {
  ...automaticDateReceipt,
  actor: {
    type: "human",
    id: "human:01a00000-0000-7000-8000-000000000022",
    label: { key: "magic.by.member", values: { name: "Kirsten Vogel" } },
  },
  source_record: {
    type: "activity",
    id: "01a00000-0000-7000-8000-000000000021",
    label:
      "Quarterly renewal call with the procurement team of Nordlicht Logistik und Lagerhaltung",
  },
  changes: [
    { field: "expected_close_date", before: "2026-09-01", after: "2026-09-27" },
    {
      field: "description",
      before: "Send the quote",
      after:
        "Send the revised quote with the volume discount the procurement team asked for",
    },
  ],
};

export const WithLongSource: Story = {
  render: () => (
    <StoryProviders>
      <OpenSourceProvider openSource={() => undefined}>
        <ReceiptDetail receipt={receipt} />
      </OpenSourceProvider>
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("Details"));
    await expect(
      canvas.getByRole("button", { name: /Quarterly renewal call/ }),
    ).toBeVisible();
  },
};

export const WithLongSourcePhone: Story = {
  ...WithLongSource,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** The date of the call Margince read, apart from when it acted, and that its
 *  reason is Margince's reading. */
export const WithSourceDateAndInference: Story = {
  render: () => (
    <StoryProviders>
      <OpenSourceProvider openSource={() => undefined}>
        <ReceiptDetail
          receipt={{
            ...receipt,
            source_occurred_at: "2026-09-10T15:30:00Z",
            reason_kind: "inference",
          }}
        />
      </OpenSourceProvider>
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("Details"));
    await expect(canvas.getByText("Source date")).toBeVisible();
    await expect(canvas.getByText(/An inference/)).toBeVisible();
  },
};
