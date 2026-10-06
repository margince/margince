// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { PurgeDialog } from "./capture-purge-dialog";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The preview answers with every count a receipt can carry, so the checked
// state shows the longest body this confirm holds.
const PREVIEW = {
  destroyed: 41,
  released: 3,
  skipped: 6,
  anonymised: 2,
  preview: true,
  kept: {
    held: 1,
    under_statute: 4,
    under_request: 1,
    under_undetermined_floor: 1,
    statutory_years: 6,
    statutory_from_year_end: true,
  },
};

function story() {
  return () => {
    installFetchStub({
      "POST /capture/exclusions/ex-2/purge": () => jsonResponse(PREVIEW),
    });
    return (
      <StoryProviders>
        <PurgeDialog
          ruleId="ex-2"
          ruleValue="recruiting.example"
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PurgeDialog> = {
  title: "Settings/You/Capture activity/Delete captured mail",
  component: PurgeDialog,
};
export default meta;
type Story = StoryObj<typeof PurgeDialog>;

export const AsItOpens: Story = { render: story() };

async function checkFirst({ canvasElement }: { canvasElement: HTMLElement }) {
  const body = within(canvasElement.ownerDocument.body);
  await body.findByRole("dialog");
  await userEvent.click(
    await body.findByRole("button", { name: "Check first" }),
  );
  await body.findByRole("button", { name: "Delete permanently" });
}

export const Checked: Story = { render: story(), play: checkFirst };

export const CheckedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
  play: checkFirst,
};
