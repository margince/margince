// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { suggestion } from "../deals/dealsuggestion.fixture";
import { HomeRepairControls } from "./homerepair";
import {
  proposalRepairReview,
  repairReview,
  suggestionRepairReview,
} from "./homerepair.fixtures";

function example(
  canActivate: boolean,
  refused = false,
  preview = repairReview,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        data_task: ["read", "update"],
        activity: ["read", "delete"],
        deal: ["read", "create"],
      }),
      "GET /approvals/*": () =>
        jsonResponse({ summary: "Prepare the account review" }),
      "GET /activities/*": () =>
        jsonResponse({ subject: "Prepare the account review" }),
      "GET /deal-suggestions/*": () => jsonResponse(suggestion),
      "POST /admin/home-repair/preview": () => jsonResponse(preview),
      "POST /admin/home-repair/activate": () =>
        refused
          ? jsonResponse({ code: "conflict", message: "Review changed." }, 409)
          : new Response(null, { status: 204 }),
    });
    return (
      <StoryProviders>
        <HomeRepairControls version={7} canActivate={canActivate} />
      </StoryProviders>
    );
  };
}
const meta = {
  title: "Settings/Governance/System health/Review automatic work",
  component: HomeRepairControls,
  args: { version: 7, canActivate: true },
} satisfies Meta<typeof HomeRepairControls>;
export default meta;
type Story = StoryObj<typeof meta>;

export const DealSuggestions: Story = {
  render: example(true, false, suggestionRepairReview),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Deal suggestions" }),
    );
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview items" }),
    );
    await expect(
      await canvas.findByText("Retire automatic deal suggestion"),
    ).toBeInTheDocument();
    await expect(
      canvas.getByRole("button", { name: "Apply 1 reviewed change" }),
    ).toBeInTheDocument();
  },
};

export const Review: Story = {
  render: example(true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview items" }),
    );
    await expect(
      await canvas.findByText("Keep for human review"),
    ).toBeInTheDocument();
    await expect(
      canvas.getByRole("button", { name: "Apply 1 reviewed change" }),
    ).toBeInTheDocument();
  },
};
export const ReadOnly: Story = {
  render: example(false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview items" }),
    );
    await expect(
      await canvas.findByText("Keep for human review"),
    ).toBeInTheDocument();
    await expect(
      canvas.queryByRole("button", { name: "Apply 1 reviewed change" }),
    ).not.toBeInTheDocument();
  },
};
export const ChangedSinceReview: Story = {
  render: example(true, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview items" }),
    );
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Apply 1 reviewed change",
      }),
    );
    await expect(await canvas.findByRole("alert")).toBeInTheDocument();
  },
};

export const DuplicateProposal: Story = {
  render: example(true, false, proposalRepairReview),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Proposals" }));
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview items" }),
    );
    await expect(
      await canvas.findByText("Withdraw automatic proposal"),
    ).toBeInTheDocument();
    await expect(
      canvas.getByText("An identical pending proposal remains available."),
    ).toBeInTheDocument();
  },
};
