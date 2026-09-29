// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { StageLadderEditor } from "./settings.stages";
import "./settings.pipelines.css";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The ladder inside one pipeline, as the pipelines page draws it: the strip of
// its shape, the open stages a reader drags into order, and the closing pair
// locked at the end. Probabilities are figures in a column of figures, so they
// wear `.t-num` and "5%" under "50%" keeps its right edge.

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];

const PIPELINE_ID = "33333333-3333-4333-8333-333333333333";

function stage(
  n: number,
  name: string,
  semantic: Stage["semantic"],
  winProbability: number,
): Stage {
  return {
    id: `44444444-4444-4444-8444-44444444444${n}`,
    pipeline_id: PIPELINE_ID,
    name,
    position: n,
    semantic,
    win_probability: winProbability,
  };
}

const sales: Pipeline = {
  id: PIPELINE_ID,
  name: "Sales",
  is_default: true,
  position: 1,
  version: 3,
  stages: [
    stage(1, "Qualified", "open", 10),
    stage(2, "Discovery", "open", 25),
    stage(3, "Proposal", "open", 50),
    stage(4, "Negotiation", "open", 75),
    stage(5, "Won", "won", 100),
    stage(6, "Lost", "lost", 0),
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

function served(pipeline: Pipeline, canEdit: boolean) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    installFetchStub({
      "GET /me": meRoute({
        pipeline: canEdit ? ["read", "update", "delete"] : ["read"],
      }),
    });
    return (
      <StoryProviders>
        <div className="pipeline-detail">
          <StageLadderEditor pipeline={pipeline} canEdit={canEdit} />
        </div>
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

/** An admin's ladder: a handle on every open stage, verbs on every row. */
export const Editable: Story = {
  render: served(sales, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const open = await canvas.findByRole("list", { name: "Open stages" });
    await expect(within(open).getByText("50%")).toHaveClass("t-num");
    // The keyboard path: the handle takes the arrow keys, and the move is said.
    const handle = within(open).getByRole("button", {
      name: "Move Proposal, step 3 of 4",
    });
    handle.focus();
    await userEvent.keyboard("{ArrowUp}");
    await expect(
      await canvas.findByText("Proposal is now step 2 of 4"),
    ).toBeInTheDocument();
  },
};

/** A reader's ladder: the same shape and figures, no handle and no verb. */
export const ReadOnly: Story = {
  render: served(sales, false),
};

/** A stage whose odds sit below the stage above it is pointed out, not refused. */
export const OddsDip: Story = {
  render: served(dipping, true),
};

/** A pipeline just created: the closing pair is in place, no open stage yet. */
export const NoOpenStages: Story = {
  render: served(fresh, true),
};

/** The ladder in dark, where the strip's shading and the outcome plates move. */
export const EditableDark: Story = {
  globals: { theme: "dark" },
  render: served(sales, true),
};
