// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { StageLadderEditor } from "./settings.stages";
import "./settings.pipelines.css";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The ladder inside one pipeline, as the pipelines page draws it. One table
// holds the open stages a reader drags into order, then the closing pair.

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];

const PIPELINE_ID = "33333333-3333-4333-8333-333333333333";
const HINT_ID = "stage-order-hint";

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

function served(pipeline: Pipeline, canEdit: boolean) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    installFetchStub({
      "GET /me": meRoute({
        pipeline: canEdit ? ["read", "update", "delete"] : ["read"],
      }),
      [`GET /stages/${DISCOVERY.id}/exit-criteria`]: () =>
        jsonResponse(CRITERIA),
    });
    return (
      <StoryProviders>
        <Panel title={pipeline.name}>
          <PanelBody>
            <PanelIntro>
              <span id={HINT_ID}>
                Drag a stage by its handle, or focus the handle and press the up
                or down arrow.
              </span>
            </PanelIntro>
          </PanelBody>
          <StageLadderEditor
            pipeline={pipeline}
            canEdit={canEdit}
            hintId={HINT_ID}
          />
        </Panel>
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
      within(table).getByRole("button", { name: "Move Proposal, step 2 of 4" }),
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
    await canvas.findByRole("table", { name: "Stages" });
    const root = canvasElement.ownerDocument.documentElement;
    await expect(root.scrollWidth).toBeLessThanOrEqual(root.clientWidth);
  },
};

/** The ladder in dark, where the outcome badge and the dip warning move. */
export const EditableDark: Story = {
  globals: { theme: "dark" },
  render: served(dipping, true),
};
