// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { PipelinesCard } from "./settings.pipelines";

// The several-pipeline catalog lives in settings.stories.tsx; this file holds
// one pipeline standing alone, its second arriving, and a refused restore.
const meta: Meta = {
  title: "Settings/Sales/Pipelines/Pipeline page",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const retiredPipeline = {
  id: "pl-old",
  name: "Legacy renewals",
  is_default: false,
  position: 1,
  archived_at: "2026-03-01T00:00:00Z",
  stages: [
    {
      id: "s1",
      pipeline_id: "pl-old",
      name: "Renew",
      position: 1,
      semantic: "open",
      win_probability: 60,
    },
  ],
};

export const RestoreRefused: Story = {
  render: () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    stubWithSession(
      {
        "GET /pipelines": () =>
          jsonResponse({
            data: [retiredPipeline],
            page: { next_cursor: null, has_more: false },
          }),
        "POST /pipelines/pl-old/restore": () =>
          jsonResponse(
            {
              type: "about:blank",
              title: "Restore refused",
              status: 409,
              detail: "A pipeline named Legacy renewals is already in use.",
            },
            409,
          ),
      },
      { pipeline: ["read", "create", "update", "delete"] },
    );
    return (
      <StoryProviders>
        <PipelinesCard />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Restore" }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "A pipeline named Legacy renewals is already in use.",
    );
  },
};

const salesPipeline = {
  id: "pl-sales",
  name: "Sales",
  is_default: true,
  position: 1,
  version: 2,
  stages: [
    ["s1", "Qualified", "open", 10],
    ["s2", "Discovery", "open", 25],
    ["s3", "Proposal", "open", 50],
    ["s4", "Won", "won", 100],
    ["s5", "Lost", "lost", 0],
  ].map(([id, name, semantic, odds], index) => ({
    id,
    pipeline_id: "pl-sales",
    name,
    position: index + 1,
    semantic,
    win_probability: odds,
    version: 1,
    ...(semantic === "open"
      ? { exit_criteria: { required: index, optional: 0 } }
      : {}),
  })),
};

type Grants = Parameters<typeof stubWithSession>[1];

function alone(allow: Grants) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    stubWithSession(
      {
        "GET /pipelines": () =>
          jsonResponse({
            data: [salesPipeline],
            page: { next_cursor: null, has_more: false },
          }),
        "GET /stages/s2/exit-criteria": () =>
          jsonResponse({
            data: [
              {
                id: "c1",
                stage_id: "s2",
                key: "budget_confirmed",
                label: "Buyer confirmed the budget",
                kind: "buyer_confirmed",
                required: true,
                position: 1,
                version: 1,
              },
            ],
          }),
        "GET /stages/s1/exit-criteria": () => jsonResponse({ data: [] }),
      },
      allow,
    );
    return (
      <StoryProviders>
        <PipelinesCard />
      </StoryProviders>
    );
  };
}

const ADMIN: Grants = { pipeline: ["read", "create", "update", "delete"] };

/** One pipeline: no catalog to choose from, so its own head offers the next one. */
export const OnePipeline: Story = {
  render: alone(ADMIN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const head = await canvas.findByRole("region", { name: "Sales" });
    await expect(
      within(head).getByRole("button", { name: /New pipeline/ }),
    ).toBeInTheDocument();
  },
};

/** A first stage with no criteria: one line and the verb that adds the first. */
export const NoCriteriaYet: Story = {
  render: alone(ADMIN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Exit criteria for Qualified",
      }),
    );
    await expect(
      await canvas.findByText("No exit criteria yet."),
    ).toBeInTheDocument();
    await expect(
      canvas.getByRole("button", { name: "Add criterion" }),
    ).toBeInTheDocument();
  },
};

/** The same page for a reader who may only look: the create verbs stay, refused with why. */
export const OnePipelineReadOnly: Story = {
  render: alone({ pipeline: ["read"] }),
};

export const OnePipelinePhone: Story = {
  render: alone(ADMIN),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table", { name: "Stages" });
    const root = canvasElement.ownerDocument.documentElement;
    await expect(root.scrollWidth).toBeLessThanOrEqual(root.clientWidth);
  },
};

export const OnePipelineDark: Story = {
  globals: { theme: "dark" },
  render: alone(ADMIN),
};

// The pipeline created here is in the next read, so the catalog appears.
function growing() {
  let catalog: readonly object[] = [salesPipeline];
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  stubWithSession(
    {
      "GET /pipelines": () =>
        jsonResponse({
          data: catalog,
          page: { next_cursor: null, has_more: false },
        }),
      "POST /pipelines": (body) => {
        const created = { id: "pl-new", version: 1, ...(body as object) };
        catalog = [...catalog, created];
        return jsonResponse(created, 201);
      },
    },
    ADMIN,
  );
  return (
    <StoryProviders>
      <PipelinesCard />
    </StoryProviders>
  );
}

/** The second pipeline: the catalog appears, and focus lands on the new name. */
export const SecondPipeline: Story = {
  render: growing,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    const head = await canvas.findByRole("region", { name: "Sales" });
    await userEvent.click(
      await within(head).findByRole("button", { name: /New pipeline/ }),
    );
    await userEvent.type(await body.findByLabelText(/Name/), "Partnerships");
    await userEvent.click(body.getByRole("button", { name: "Create" }));
    const name = await canvas.findByRole("heading", { name: "Partnerships" });
    await waitFor(() =>
      expect(canvasElement.ownerDocument.activeElement).toBe(
        name.firstElementChild,
      ),
    );
  },
};
