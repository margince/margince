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
import { HomeRepairControls } from "./homerepair";
import { repairReview } from "./homerepair.fixtures";

function example(canActivate: boolean, refused = false) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        data_task: ["read", "update"],
        activity: ["read", "delete"],
      }),
      "GET /activities/*": () =>
        jsonResponse({ subject: "Prepare the account review" }),
      "POST /admin/home-repair/preview": () => jsonResponse(repairReview),
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

export const Review: Story = {
  render: example(true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview tasks" }),
    );
    await expect(
      await canvas.findByText("Keep for human review"),
    ).toBeInTheDocument();
    await expect(
      canvas.getByRole("button", { name: "Archive 1 reviewed reminders" }),
    ).toBeInTheDocument();
  },
};
export const ReadOnly: Story = {
  render: example(false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview tasks" }),
    );
    await expect(
      await canvas.findByText("Keep for human review"),
    ).toBeInTheDocument();
    await expect(
      canvas.queryByRole("button", { name: "Archive 1 reviewed reminders" }),
    ).not.toBeInTheDocument();
  },
};
export const ChangedSinceReview: Story = {
  render: example(true, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "Preview tasks" }),
    );
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Archive 1 reviewed reminders",
      }),
    );
    await expect(await canvas.findByRole("alert")).toBeInTheDocument();
  },
};
