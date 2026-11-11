// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, waitFor } from "storybook/test";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { paragraphsFrom } from "../../design-system/richtext";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { WordingCorrectionModal } from "./wordingcorrectionmodal";

const meta: Meta = { title: "Patterns/Approved wording correction" };
export default meta;
type Story = StoryObj;

const approved: components["schemas"]["WordingCorrectionContext"] = {
  instruction_id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  version: 7,
  subject: "Requested information",
  body: "Thank yuo for your help. The requested information is attached.",
  html_body: "",
  wording_digest: "a".repeat(64),
  reason_code: "contractual_necessity",
  explanation: "The contract requires this notice.",
  warning_version: "override-v1",
};

function Correction({
  context = approved,
}: Readonly<{ context?: typeof approved }>) {
  const [open, setOpen] = useState(true);
  installFetchStub({
    "POST /communication-reviews/review-1/correct-wording": () =>
      jsonResponse({ version: 8 }),
  });
  return (
    <StoryProviders>
      <Button onClick={() => setOpen(true)}>Open correction</Button>
      {open && (
        <WordingCorrectionModal
          reviewId="review-1"
          context={context}
          onClose={() => setOpen(false)}
          onSaved={() => setOpen(false)}
        />
      )}
    </StoryProviders>
  );
}

const opensCorrection = async () => {
  await waitFor(() => expect(screen.getByRole("dialog")).toBeVisible());
  expect(
    screen.getByRole("button", { name: "Save correction" }),
  ).toBeDisabled();
};

export const Spelling: Story = {
  render: () => <Correction />,
  play: opensCorrection,
};

export const LongMessage: Story = {
  render: () => {
    const body = Array.from(
      { length: 12 },
      (_, index) =>
        `Section ${index + 1}. The requested informaton remains available for your review.`,
    ).join("\n\n");
    return (
      <Correction
        context={{ ...approved, body, html_body: paragraphsFrom(body) }}
      />
    );
  },
  play: opensCorrection,
};

export const SpellingPhone: Story = {
  ...Spelling,
  tags: ["uat-phone"],
};

export const LongMessagePhone: Story = {
  ...LongMessage,
  tags: ["uat-phone"],
};
