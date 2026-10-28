// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { suggestion } from "../../testing/dealsuggestion.fixture";
import { ReviewActionsProvider } from "../../testing/reviewactions";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { HomeRepairControls } from "./homerepair";
import {
  classificationRepairReview,
  proposalRepairReview,
  provenanceRepairReview,
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
        activity: ["read", "update", "delete"],
        deal: ["read", "create"],
      }),
      ...Object.fromEntries(
        preview.tasks.map((task) => [
          `GET /${task.source === "suggestion" ? "deal-suggestions" : task.source === "approval" ? "approvals" : "activities"}/${task.id}`,
          () =>
            jsonResponse(
              task.source === "suggestion"
                ? suggestion
                : {
                    id: task.id,
                    subject: "Prepare the account review",
                    summary: "Prepare the account review",
                  },
            ),
        ]),
      ),
      "POST /admin/home-repair/preview": () => jsonResponse(preview),
      "POST /admin/home-repair/activate": () =>
        refused
          ? jsonResponse({ code: "conflict", message: "Review changed." }, 409)
          : new Response(null, { status: 204 }),
    });
    return (
      <StoryProviders>
        <ReviewActionsProvider>
          <HomeRepairControls version={7} canActivate={canActivate} />
        </ReviewActionsProvider>
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

export const MessageClassification: Story = {
  render: example(true, false, classificationRepairReview),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Message results" }),
    );
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview items" }),
    );
    await expect(
      await canvas.findByText(
        "Current: Requests action. Proposed: Information only.",
      ),
    ).toBeInTheDocument();
    await expect(
      await canvas.findByText("Prepare the account review"),
    ).toBeInTheDocument();
    await expect(canvas.getByRole("button", { name: "Inspect" })).toBeEnabled();
    await expect(
      canvas.getByRole("button", { name: "Apply 1 reviewed change" }),
    ).toBeInTheDocument();
  },
};

export const SourceAndGeneratedDate: Story = {
  render: example(true, false, provenanceRepairReview),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview items" }),
    );
    await expect(
      await canvas.findByText("Correct source or generated date"),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Apply 1 reviewed change" }),
    ).toBeVisible();
  },
};
export const SourceAndGeneratedDateDark: Story = {
  ...SourceAndGeneratedDate,
  globals: { theme: "dark" },
};
