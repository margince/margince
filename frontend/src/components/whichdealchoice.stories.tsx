// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  permissionDenied,
  StoryProviders,
} from "../testing/story-utils";
import { WhichDealChoice } from "./whichdealchoice";

const QUESTION: components["schemas"]["Activity"] = {
  id: "task-1",
  kind: "task",
  writable: true,
  subject: "Which deal is this email about?",
  occurred_at: "2026-09-01T09:12:00Z",
  occurred_precision: "instant",
  offers_email: false,
  created_at: "2026-09-01T09:12:00Z",
  updated_at: "2026-09-01T09:12:00Z",
  captured_by: "system:capture-which-deal",
  source: "which_deal",
  source_system: "which_deal",
  source_activity_id: "mail-1",
  raw: { offered_deal_ids: ["d-1", "d-2", "d-3"] },
  is_done: false,
  version: 1,
  content_state: "available",
  audience: "workspace",
};

type DealFixture = Pick<
  components["schemas"]["Deal"],
  "id" | "name" | "status" | "amount_minor" | "currency"
>;

const RENEWAL: DealFixture = {
  id: "d-1",
  name: "Nordwind renewal",
  status: "open",
  amount_minor: 4_200_000,
  currency: "EUR",
};
const PILOT: DealFixture = {
  id: "d-2",
  name: "Nordwind pilot",
  status: "open",
  amount_minor: 850_000,
  currency: "EUR",
};
const CLOSED: DealFixture = {
  id: "d-3",
  name: "Nordwind previous contract",
  status: "won",
  amount_minor: 2_000_000,
  currency: "EUR",
};

function Frame({
  state = "resolved",
}: Readonly<{ state?: "resolved" | "empty" | "pending" | "failed" }>) {
  const read = (deal: DealFixture): Response | Promise<Response> => {
    if (state === "pending") return new Promise<Response>(() => {});
    if (state === "failed") return permissionDenied();
    return jsonResponse(state === "empty" ? { ...deal, status: "won" } : deal);
  };
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"], deal: ["read"] }),
    "GET /deals/d-1": () => read(RENEWAL),
    "GET /deals/d-2": () => read(PILOT),
    "GET /deals/d-3": () => read(CLOSED),
    "POST /activities/relink-thread": () => jsonResponse({}),
    "POST /activities/mail-1/deal-decline": () =>
      jsonResponse({
        declined_deal_ids: ["d-1", "d-2", "d-3"],
        unlinked: false,
        project_unfiled: false,
        project_kept: false,
        questions_closed: 1,
      }),
  });
  return (
    <StoryProviders>
      <WhichDealChoice task={QUESTION} />
    </StoryProviders>
  );
}

const meta: Meta<typeof Frame> = {
  title: "Patterns/Task deal choice",
  component: Frame,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Frame>;

export const Resolved: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.findAllByRole("radio")).resolves.toHaveLength(2);
    await expect(canvas.queryByText(CLOSED.name)).not.toBeInTheDocument();
    await expect(
      canvas.getByRole("button", { name: en["tasks.whichDeal.file"] }),
    ).toBeDisabled();
  },
};

export const Selected: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const renewal = await canvas.findByRole("radio", {
      name: /Nordwind renewal/,
    });
    await userEvent.click(renewal);
    await expect(renewal).toBeChecked();
    await expect(
      canvas.getByRole("button", { name: en["tasks.whichDeal.file"] }),
    ).toBeEnabled();
  },
};

export const EmptyChoices: Story = {
  args: { state: "empty" },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.findByRole("button", { name: en["tasks.whichDeal.none"] }),
    ).resolves.toBeVisible();
    await expect(canvas.queryAllByRole("radio")).toHaveLength(0);
    await expect(
      canvas.queryByRole("button", { name: en["tasks.whichDeal.file"] }),
    ).not.toBeInTheDocument();
  },
};

export const Pending: Story = { args: { state: "pending" } };

export const Failed: Story = {
  args: { state: "failed" },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.findByRole("button", { name: en["common.retry"] }),
    ).resolves.toBeVisible();
    await expect(canvas.getByRole("alert")).toBeVisible();
    await expect(canvas.queryAllByRole("radio")).toHaveLength(0);
  },
};
