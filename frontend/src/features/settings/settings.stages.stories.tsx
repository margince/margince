// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQuery } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Panel } from "../../design-system/panel";
import { unwrap } from "../../lib/problem";
import { SETTINGS_PIPELINES } from "./settings.pipelineorder";
import { StageLadderEditor } from "./settings.stages";
import "./settings.pipelines.css";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";

// The ladder inside one pipeline, as the pipelines page draws it. One table
// holds the open stages a reader drags into order, then the closing pair.

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];

const PIPELINE_ID = "33333333-3333-4333-8333-333333333333";

function stage(
  n: number,
  name: string,
  semantic: Stage["semantic"],
  winProbability: number,
  extra: Partial<Stage> = {},
): Stage {
  return {
    id: `44444444-4444-4444-8444-44444444444${n}`,
    pipeline_id: PIPELINE_ID,
    name,
    position: n,
    semantic,
    win_probability: winProbability,
    version: 1,
    ...(semantic === "open"
      ? { exit_criteria: { required: 0, optional: 0 } }
      : {}),
    ...extra,
  };
}

const DISCOVERY = stage(2, "Discovery", "open", 25, {
  exit_criteria: { required: 2, optional: 1 },
  deal_count: 3,
});

const sales: Pipeline = {
  id: PIPELINE_ID,
  name: "Sales",
  is_default: true,
  position: 1,
  version: 3,
  stages: [
    stage(1, "Qualified", "open", 10, { deal_count: 0 }),
    DISCOVERY,
    stage(3, "Proposal", "open", 50, {
      exit_criteria: { required: 1, optional: 0 },
    }),
    stage(4, "Negotiation", "open", 75),
    stage(5, "Won", "won", 100),
    stage(6, "Closed lost", "lost", 0),
  ],
};

// Proposal dragged above Discovery without its odds changing: the ladder dips,
// and the row that dips says so without refusing the order.
const dipping: Pipeline = {
  ...sales,
  stages: [
    stage(1, "Qualified", "open", 10),
    stage(2, "Proposal", "open", 50),
    stage(3, "Discovery", "open", 25),
    stage(4, "Negotiation", "open", 75),
    stage(5, "Won", "won", 100),
    stage(6, "Lost", "lost", 0),
  ],
};

const fresh: Pipeline = {
  ...sales,
  name: "Partner deals",
  is_default: false,
  stages: [stage(1, "Won", "won", 100), stage(2, "Lost", "lost", 0)],
};

const CRITERIA = {
  data: [
    {
      id: "55555555-5555-4555-8555-555555555551",
      stage_id: DISCOVERY.id,
      key: "budget_confirmed",
      label: "Buyer confirmed the budget",
      kind: "buyer_confirmed",
      required: true,
      position: 1,
      version: 1,
    },
  ],
  page: { next_cursor: null },
};

function stageIdsIn(body: unknown): string[] {
  if (typeof body !== "object" || body === null || !("stage_ids" in body)) {
    return [];
  }
  const ids = body.stage_ids;
  return Array.isArray(ids)
    ? ids.filter((id): id is string => typeof id === "string")
    : [];
}

// The server keeps the order it was sent and moves every version. The re-read
// after a save then draws what was saved.
function reordered(pipeline: Pipeline, ids: readonly string[]): Pipeline {
  const stages = pipeline.stages ?? [];
  return {
    ...pipeline,
    version: (pipeline.version ?? 0) + 1,
    stages: ids.flatMap((id, index) => {
      const found = stages.find((each) => each.id === id);
      return found
        ? [{ ...found, position: index + 1, version: (found.version ?? 0) + 1 }]
        : [];
    }),
  };
}

// The ladder reads its pipeline from the page's own query, as the page does.
// An optimistic order and the re-read after it both land there.
function LiveLadder({
  id,
  canEdit,
}: Readonly<{ id: string; canEdit: boolean }>) {
  const query = useQuery({
    queryKey: SETTINGS_PIPELINES,
    queryFn: async () =>
      unwrap(
        await api.GET("/pipelines", {
          params: { query: { include_archived: true } },
        }),
      ).data,
  });
  const pipeline = query.data?.find((each) => each.id === id);
  if (!pipeline) {
    return null;
  }
  return (
    <Panel title={pipeline.name}>
      <StageLadderEditor pipeline={pipeline} canEdit={canEdit} />
    </Panel>
  );
}

function served(pipeline: Pipeline, canEdit: boolean) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    let current = pipeline;
    installFetchStub({
      "GET /me": meRoute({
        pipeline: canEdit ? ["read", "update", "delete"] : ["read"],
      }),
      "GET /pipelines": () =>
        jsonResponse({
          data: [current],
          page: { next_cursor: null, has_more: false },
        }),
      [`PUT /pipelines/${pipeline.id}/stage-order`]: (body) => {
        current = reordered(current, stageIdsIn(body));
        return jsonResponse(current);
      },
      [`GET /stages/${DISCOVERY.id}/exit-criteria`]: () =>
        jsonResponse(CRITERIA),
    });
    return (
      <StoryProviders>
        <LiveLadder id={pipeline.id} canEdit={canEdit} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof StageLadderEditor> = {
  title: "Settings/Sales/Pipelines/Stage ladder",
  component: StageLadderEditor,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof StageLadderEditor>;

/** An admin's ladder: a grip on every open stage, a menu on every row. */
export const Editable: Story = {
  render: served(sales, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const table = await canvas.findByRole("table", { name: "Stages" });
    // The grip takes the arrow keys, the move is said, and focus follows.
    const grip = within(table).getByRole("button", {
      name: "Move Proposal, step 3 of 4",
    });
    grip.focus();
    await userEvent.keyboard("{ArrowUp}");
    await expect(
      await canvas.findByText("Proposal is now step 2 of 4"),
    ).toBeInTheDocument();
    await expect(
      await within(table).findByRole("button", {
        name: "Move Proposal, step 2 of 4",
      }),
    ).toHaveFocus();
  },
};

/** A reader's ladder: the same shape and figures, no grip and no menu. */
export const ReadOnly: Story = {
  render: served(sales, false),
};

/** The row menu of a stage that holds deals: Remove is refused and says why. */
export const OccupiedRowMenu: Story = {
  render: served(sales, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table", { name: "Stages" });
    await userEvent.click(
      canvas.getByRole("button", { name: "Actions for Discovery" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await expect(page.getByRole("button", { name: "Remove" })).toBeDisabled();
    await expect(
      page.getByText("3 deals are in this stage"),
    ).toBeInTheDocument();
  },
};

/** A stage's exit criteria, opened in place under its row. */
export const CriteriaOpen: Story = {
  render: served(sales, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table", { name: "Stages" });
    await userEvent.click(
      canvas.getByRole("button", { name: "Exit criteria for Discovery" }),
    );
    await expect(
      await canvas.findByText("Buyer confirmed the budget"),
    ).toBeInTheDocument();
  },
};

/** A reader's criteria: the rows and their standing, with no verb to change them. */
export const CriteriaReadOnly: Story = {
  render: served(sales, false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table", { name: "Stages" });
    await userEvent.click(
      canvas.getByRole("button", { name: "Exit criteria for Discovery" }),
    );
    await expect(
      await canvas.findByText("Buyer confirmed the budget"),
    ).toBeInTheDocument();
    await expect(
      canvas.queryByRole("button", { name: "Add criterion" }),
    ).toBeNull();
    await expect(canvas.queryByRole("button", { name: /^Remove / })).toBeNull();
  },
};

/** One stage's criteria open at a time; adding a criterion opens its stage. */
export const OneStageOpen: Story = {
  render: served(sales, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table", { name: "Stages" });
    const discovery = canvas.getByRole("button", {
      name: "Exit criteria for Discovery",
    });
    const proposal = canvas.getByRole("button", {
      name: "Exit criteria for Proposal",
    });
    await userEvent.click(discovery);
    await userEvent.click(proposal);
    await expect(proposal).toHaveAttribute("aria-expanded", "true");
    await expect(discovery).toHaveAttribute("aria-expanded", "false");
    const actions = canvas.getByRole("button", {
      name: "Actions for Discovery",
    });
    await userEvent.click(actions);
    const menu = canvasElement.ownerDocument.getElementById(
      actions.getAttribute("aria-controls") ?? "",
    );
    if (!menu) throw new Error("the row menu named no panel");
    await userEvent.click(
      within(menu).getByRole("button", { name: "Add criterion" }),
    );
    await expect(discovery).toHaveAttribute("aria-expanded", "true");
    await expect(proposal).toHaveAttribute("aria-expanded", "false");
  },
};

/** A stage whose odds sit below the stage above it is pointed out, not refused. */
export const OddsDip: Story = {
  render: served(dipping, true),
};

/** A pipeline just created: the closing pair is in place, no open stage yet. */
export const NoOpenStages: Story = {
  render: served(fresh, true),
};

/** At a phone's width each row folds: grip, name and menu, then the figures. */
export const EditablePhone: Story = {
  render: served(sales, true),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const table = await canvas.findByRole("table", { name: "Stages" });
    const row = within(table).getByTestId(`stage-${DISCOVERY.id}`);
    const title = row.querySelector('[data-fold="title"]');
    const rest = row.querySelector('[data-fold="rest"]');
    await expect(title).not.toBeNull();
    await expect(rest).not.toBeNull();
    await expect(rest?.getBoundingClientRect().top).toBeGreaterThanOrEqual(
      title?.getBoundingClientRect().bottom ?? Number.POSITIVE_INFINITY,
    );
    const root = canvasElement.ownerDocument.documentElement;
    await expect(root.scrollWidth).toBeLessThanOrEqual(root.clientWidth);
  },
};

/** The ladder in dark, where the outcome badge and the dip warning move. */
export const EditableDark: Story = {
  globals: { theme: "dark" },
  render: served(dipping, true),
};
