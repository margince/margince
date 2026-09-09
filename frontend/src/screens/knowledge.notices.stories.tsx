// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  IngestFailure,
  ReindexingNotice,
  UploadRefusals,
} from "./knowledge.notices";
import { StoryProviders } from "./story-utils";

// What the document-sets card says about itself. The refusal list is the one
// worth reading side by side with the rest: it is a single heading over a list
// of names, where the card once stacked one whole notice per refused file.

const meta: Meta = {
  title: "Patterns/Document set notices",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** A set being re-read after an indexing change. Nothing is lost, and there is
 *  nothing to press. */
export const Reindexing: Story = {
  render: () => (
    <StoryProviders>
      <ReindexingNotice />
    </StoryProviders>
  ),
};

/** Why one filed document could not be read into passages. */
export const IngestFailed: Story = {
  render: () => (
    <StoryProviders>
      <IngestFailure detail="No text could be extracted: the pages are scanned images." />
    </StoryProviders>
  ),
};

/** One file the drop left behind. The singular heading, and one name. */
export const OneRefusedFile: Story = {
  render: () => (
    <StoryProviders>
      <UploadRefusals
        refusals={[
          { id: "a", filename: "handbook.pdf", message: "PDFs are refused." },
        ]}
      />
    </StoryProviders>
  ),
};

/** Three of them: one heading, three names, and the reader compares nothing. */
export const SeveralRefusedFiles: Story = {
  render: () => (
    <StoryProviders>
      <UploadRefusals
        refusals={[
          { id: "a", filename: "handbook.pdf", message: "PDFs are refused." },
          {
            id: "b",
            filename: "notes.docx",
            message: "Word files are refused.",
          },
          {
            id: "c",
            filename: "policy.md",
            message: "Already filed as policy-2026.md.",
          },
        ]}
      />
    </StoryProviders>
  ),
};
