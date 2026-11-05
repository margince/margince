// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { OpenSourceProvider } from "../../lib/opensource";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "../../testing/worklist.receiptreview.fixtures";
import { BriefChanges } from "./brief.changes";
import "./brief.css";

const meta: Meta<typeof BriefChanges> = {
  title: "Shell/Home/Changes",
  component: BriefChanges,
};
export default meta;
type Story = StoryObj<typeof BriefChanges>;
export const AppliedChanges: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /worklist/handled": () =>
          jsonResponse({
            as_of: "2026-09-13T08:00:00Z",
            receipts: [automaticDateReceipt, automaticStageReceipt],
            truncated: false,
          }),
        [`GET /deals/${automaticStageReceipt.subject?.id}`]: () =>
          jsonResponse({ name: "PIM Rollout" }),
      },
      { deal: ["read", "update"] },
    );
    // A group of Home's receipt, so it is drawn inside a pane the way the
    // receipt draws it.
    return (
      <StoryProviders>
        <Panel>
          <BriefChanges />
        </Panel>
      </StoryProviders>
    );
  },
};

const CALL = "01a00000-0000-7000-8000-000000000021";

const explainedReceipt = {
  ...automaticDateReceipt,
  actor: {
    type: "human" as const,
    id: "human:01a00000-0000-7000-8000-000000000022",
    label: { key: "magic.by.member", values: { name: "Kirsten Vogel" } },
  },
  source_record: {
    type: "activity",
    id: CALL,
    label:
      "Quarterly renewal call with the procurement team of Nordlicht Logistik und Lagerhaltung",
  },
  changes: [
    {
      field: "expected_close_date",
      before: "2026-09-01",
      after: "2026-09-27",
    },
    {
      field: "description",
      before: "Send the quote",
      after:
        "Send the revised quote with the volume discount the procurement team asked for",
    },
  ],
};

// One receipt opened under its line: who acted, its source and each value it
// moved, at the width the sentence above it holds.
export const OpenedReceipt: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /worklist/handled": () =>
          jsonResponse({
            as_of: "2026-09-13T08:00:00Z",
            receipts: [explainedReceipt, automaticStageReceipt],
            truncated: false,
          }),
        [`GET /deals/${automaticStageReceipt.subject?.id}`]: () =>
          jsonResponse({ name: "PIM Rollout" }),
      },
      { deal: ["read", "update"] },
    );
    return (
      <StoryProviders>
        <OpenSourceProvider openSource={() => undefined}>
          <Panel>
            <BriefChanges />
          </Panel>
        </OpenSourceProvider>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("Details"));
    await expect(
      canvas.getByRole("button", { name: /Quarterly renewal call/ }),
    ).toBeVisible();
    const line = canvasElement.querySelector(".magic-line:has(.disclosure)");
    const sentence = line?.querySelector(".magic-line-text");
    const detail = line?.querySelector(".disclosure");
    if (!sentence || !detail) {
      throw new Error("the opened receipt drew outside its line");
    }
    const sentenceBox = sentence.getBoundingClientRect();
    const detailBox = detail.getBoundingClientRect();
    await expect(Math.round(detailBox.left)).toBe(Math.round(sentenceBox.left));
    await expect(Math.round(detailBox.width)).toBeGreaterThanOrEqual(
      Math.round(sentenceBox.width),
    );
  },
};

export const OpenedReceiptDark: Story = {
  ...OpenedReceipt,
  globals: { theme: "dark" },
};

export const OpenedReceiptPhone: Story = {
  ...OpenedReceipt,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
