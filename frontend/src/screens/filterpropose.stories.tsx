// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import type { FilterVocabulary } from "./filterdata";
import { useFilterDraft } from "./filterdraft";
import { FilterEditor } from "./filtereditor";
import {
  PlainWordsFilter,
  type UnusedPhrase,
  usePlainWords,
} from "./filterpropose";
import { type Group, newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Describing a filter in plain words. The answer lands as rows marked as
// proposed, with the bar above them that keeps, replaces or undoes them; the
// states worth seeing are the wait, the landing on an empty filter and over
// the reader's own row, a proposed row the reader made their own, an
// installation with no model, and the phrases an answer could not use.
const meta: Meta<typeof PlainWordsFilter> = {
  title: "Patterns/Plain-words filter",
  component: PlainWordsFilter,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof PlainWordsFilter>;
type Canvas = ReturnType<typeof within>;

const VOCABULARY: FilterVocabulary = {
  resource: "company",
  fields: [
    {
      name: "country",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
    {
      name: "last_activity_at",
      type: "date",
      operators: ["eq", "neq", "gt", "gte", "lt", "lte", "exists"],
      custom: false,
    },
  ],
};

const UNUSED: UnusedPhrase[] = [
  {
    phrase: "likely to buy",
    code: "not_expressible",
    reason: "No field records a prediction of buying.",
  },
  {
    phrase: "hot lead",
    code: "value_not_allowed",
    reason: "not an option",
    field: "last_activity_at",
  },
];

const PROPOSED = {
  and: [
    { field: "country", op: "eq", value: "Germany" },
    { field: "last_activity_at", op: "lt", value: { days_ago: 45 } },
  ],
};

type Answer = "proposal" | "held" | "noModel" | "nothingUsable";

function routes(answer: Answer): void {
  installFetchStub({
    "POST /filters/propose": () => {
      switch (answer) {
        case "held":
          return new Promise<Response>(() => undefined);
        case "noModel":
          return jsonResponse(
            { code: "ai_not_configured", status: 409, detail: "no model" },
            409,
          );
        case "nothingUsable":
          return jsonResponse({
            resource: "company",
            filter: null,
            unsupported: UNUSED,
          });
        default:
          return jsonResponse({
            resource: "company",
            filter: PROPOSED,
            unsupported: [],
          });
      }
    },
  });
}

/** The editing panel as the page draws it, with the ask held by its parent. */
function Surface({ start }: Readonly<{ start: () => Group }>) {
  const [draft, dispatch] = useFilterDraft(start);
  const words = usePlainWords({ resource: "company", dispatch });
  return (
    <Panel title="Find companies where…">
      <PanelBody>
        <FilterEditor
          draft={draft}
          dispatch={dispatch}
          vocabulary={{ data: VOCABULARY, isPending: false, isError: false }}
          words={words}
          records="companies"
        />
      </PanelBody>
    </Panel>
  );
}

function story(answer: Answer, start: () => Group = () => newGroup("and")) {
  return () => {
    routes(answer);
    return (
      <StoryProviders>
        <Surface start={start} />
      </StoryProviders>
    );
  };
}

/** Types a description and sends it, opening the folded box first if shut. */
async function askInWords(canvas: Canvas) {
  const folded = canvas.queryByText("Describe changes in plain words");
  if (folded) {
    await userEvent.click(folded);
  }
  await userEvent.type(
    await canvas.findByLabelText("Describe the companies you want"),
    "German companies, quiet for 45 days",
  );
  await userEvent.click(
    canvas.getByRole("button", { name: "Propose conditions" }),
  );
}

const ownCondition = () =>
  newGroup("and", [newLeaf("country", "eq", "Austria")]);

// The model is reading: the button says so and the indigo wait stands under
// the description.
export const CalmStartAsking: Story = {
  render: story("held"),
  play: async ({ canvasElement }) => {
    await askInWords(within(canvasElement));
  },
};

// Proposed on the calm start: two dashed rows, and a bar offering Keep all
// and Undo, which returns to the calm start.
export const LandedOnEmpty: Story = {
  render: story("proposal"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await askInWords(canvas);
    await canvas.findByText("Keep all");
  },
};

// Over the reader's own condition the proposal is appended, and the bar also
// offers to replace what the reader had.
export const LandedOverOwnConditions: Story = {
  render: story("proposal", ownCondition),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await askInWords(canvas);
    await canvas.findByText("Replace my conditions");
  },
};

// The reader changed one proposed row: its dashes are gone and the bar counts
// the one proposal left.
export const OneRowEdited: Story = {
  render: story("proposal"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await askInWords(canvas);
    await canvas.findByText("Keep all");
    await userEvent.type(canvas.getAllByLabelText("Value")[0], " and Austria");
  },
};

// No model is configured: the description gives way to the line saying so,
// and building by hand still works.
export const NoModel: Story = {
  render: story("noModel"),
  play: async ({ canvasElement }) => {
    await askInWords(within(canvasElement));
  },
};

// An answer that expressed nothing leaves the calm start and names every
// phrase it could not use, and why.
export const CouldNotUse: Story = {
  render: story("nothingUsable"),
  play: async ({ canvasElement }) => {
    await askInWords(within(canvasElement));
  },
};
