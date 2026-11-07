// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
import { browserZone } from "../format/timezone";
import { SourceHost } from "../testing/sourcehost";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { SourceReviewDialog } from "./sourcereviewdialog";
import {
  type SourceEvidenceRef,
  SourceReviewEvidence,
} from "./sourcereviewevidence";

const evidence = {
  reference: { kind: "activity", id: "meeting-source" },
  kind: "meeting",
  title: "Original booking",
  record_type: "activity",
  record_id: "meeting-source",
  occurred_at: "2026-01-01T10:00:00Z",
  recorded_at: "2026-01-01T10:00:00Z",
};
const meta: Meta<typeof SourceReviewDialog> = {
  title: "Components/Overlays and layering/Source review",
  component: SourceReviewDialog,
  decorators: [
    (Story) => {
      stubWithSession(
        {
          "GET /privacy/notice-case-reviews/source-case": () =>
            jsonResponse({
              id: "source-case",
              contact_id: "contact-source",
              state: "source_review",
              rule: "unassessed",
              acquisition: {
                kind: "unknown_legacy",
                captured_at: "2026-01-01T10:00:00Z",
                captured_by: "system",
              },
            }),
          "GET /privacy/notice-case-reviews/source-case/source": () =>
            jsonResponse({ items: [evidence] }),
          "POST /privacy/notice-case-reviews/source-case/source/preview": () =>
            jsonResponse({
              preview_token: "a".repeat(64),
              rule: "none",
              notice_required: false,
              due_at: null,
              timezone: browserZone(),
              deadline_is_upper_bound: false,
            }),
        },
        {
          privacy_request: ["read", "update"],
          contact: ["read"],
          activity: ["read"],
        },
      );
      return (
        <StoryProviders>
          <SourceHost>
            <Story />
          </SourceHost>
        </StoryProviders>
      );
    },
  ],
  args: { caseId: "source-case", open: true, onClose: () => {} },
};
export default meta;
type Story = StoryObj<typeof SourceReviewDialog>;
export const Assessment: Story = {};
export const SelectedSourcePreview: Story = {
  play: async ({ canvasElement }) => {
    const panel = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await panel.findByRole("radio", { name: /Original booking/ }),
    );
    await userEvent.click(
      panel.getByRole("radio", { name: "Met face to face" }),
    );
    await userEvent.type(
      panel.getByRole("textbox", { name: "Obtained on" }),
      "01/01/2026",
    );
    await userEvent.tab();
    await panel.findByText(/No separate notice duty follows from this source/);
  },
};
export const SelectedSourcePreviewDark: Story = {
  ...SelectedSourcePreview,
  globals: { theme: "dark" },
};

function EvidenceChoices() {
  const [selected, setSelected] = useState<SourceEvidenceRef | null>(null);
  return (
    <SourceReviewEvidence
      caseId="source-case"
      originalKind="unknown_legacy"
      selected={selected}
      onSelect={setSelected}
    />
  );
}
export const ExistingRecords: Story = { render: () => <EvidenceChoices /> };
