// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import "./settings.css";
import { StageRow } from "./settings.stages";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The stage ladder inside one pipeline, as the pipelines card draws it: a name,
// the semantic badge, the win probability and the row verbs. The probability is
// a figure in a column of figures, so it wears `.t-num` — tabular digits in the
// body face — and "5%" under "50%" under "100%" keeps its right edge.

type Stage = components["schemas"]["Stage"];

const PIPELINE_ID = "33333333-3333-4333-8333-333333333333";

const ladder: Stage[] = [
  {
    id: "44444444-4444-4444-8444-444444444441",
    pipeline_id: PIPELINE_ID,
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 5,
  },
  {
    id: "44444444-4444-4444-8444-444444444442",
    pipeline_id: PIPELINE_ID,
    name: "Proposal",
    position: 2,
    semantic: "open",
    win_probability: 50,
  },
  {
    id: "44444444-4444-4444-8444-444444444443",
    pipeline_id: PIPELINE_ID,
    name: "Closed won",
    position: 3,
    semantic: "won",
    win_probability: 100,
  },
  {
    id: "44444444-4444-4444-8444-444444444444",
    pipeline_id: PIPELINE_ID,
    name: "Closed lost",
    position: 4,
    semantic: "lost",
    win_probability: 0,
  },
];

// `StageRow` takes the caller's translator, so the ladder is its own component
// mounted inside the providers rather than rows rendered at the story's root.
function Ladder({ canEdit }: Readonly<{ canEdit: boolean }>) {
  const t = useT();
  return (
    <ul className="stage-rows" aria-label="Stages">
      {ladder.map((stage) => (
        <StageRow
          key={stage.id}
          stage={stage}
          canEdit={canEdit}
          t={t}
          returnFocusTo={() => null}
        />
      ))}
    </ul>
  );
}

// useMe() fails fast without a workspace slug, which would collapse the admin
// ladder into the reader's — seed it so /me resolves and the verbs render. The
// criteria under each row stay folded, and answer from the stub's empty page.
function served(canEdit: boolean) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    installFetchStub({
      "GET /me": meRoute({
        pipeline: canEdit ? ["read", "update", "delete"] : ["read"],
      }),
    });
    return (
      <StoryProviders>
        <Ladder canEdit={canEdit} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof StageRow> = {
  title: "Settings/Sales/Pipelines/Stage ladder",
  component: StageRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof StageRow>;

/** An admin's ladder: every row carries its verbs. */
export const Editable: Story = {
  render: served(true),
  play: async ({ canvasElement }) => {
    const list = await within(canvasElement).findByRole("list", {
      name: "Stages",
    });
    const probability = within(list).getByText("50%");
    await expect(probability).toHaveClass("t-num");
  },
};

/** A reader's ladder: the same figures, no verbs to act on them. */
export const ReadOnly: Story = {
  render: served(false),
};

/** The ladder in dark, where the semantic badges sit on the card's own ground. */
export const EditableDark: Story = {
  globals: { theme: "dark" },
  render: served(true),
};
