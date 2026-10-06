// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { Panel, PanelBody } from "../design-system/panel";
import type { FilterVocabulary } from "./filterdata";
import { useFilterDraft } from "./filterdraft";
import { FilterEditor } from "./filtereditor";
import { type UnusedPhrase, usePlainWords } from "./filterpropose";
import { type Group, newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, StoryProviders } from "./story-utils";

// The editing panel's body. It grows with the filter: two ways in while
// there is no condition, then the rows with the description folded above
// them and a proposal's bar above the rows it marked. What a description
// could not use is said in both states.
const meta: Meta<typeof FilterEditor> = {
  title: "Patterns/Filters and views/Filter editor",
  component: FilterEditor,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof FilterEditor>;

const VOCABULARY: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "city",
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
    phrase: "decision makers only",
    code: "not_expressible",
    reason: "No field records who makes the decision.",
  },
];

/** A proposal as it lands: what a description answered, marked on the rows. */
type Landed = Readonly<{ text: string; proposed: () => Group }>;

// One array for every render, so the effect below runs once rather than on
// each render a landed proposal causes.
const NO_PHRASES: readonly UnusedPhrase[] = [];

function Editor({
  start,
  unused = NO_PHRASES,
  landed,
  noModel = false,
}: Readonly<{
  start: () => Group;
  unused?: readonly UnusedPhrase[];
  landed?: Landed;
  /** The installation answered that it has no model. */
  noModel?: boolean;
}>) {
  const [draft, dispatch] = useFilterDraft(start);
  const words = usePlainWords({ resource: "contact", dispatch });
  // What an answer does, without a model to ask: the phrases alone, or the
  // proposal landed on the rows.
  useEffect(() => {
    if (unused.length > 0) {
      dispatch({ type: "unused", unused });
    }
    if (landed) {
      dispatch({
        type: "answer",
        proposed: landed.proposed(),
        unused: [],
        text: landed.text,
      });
    }
  }, [dispatch, unused, landed]);
  return (
    <Panel title="Find contacts where…">
      <PanelBody>
        <FilterEditor
          draft={draft}
          dispatch={dispatch}
          vocabulary={{ data: VOCABULARY, isPending: false, isError: false }}
          words={noModel ? { ...words, noModel } : words}
          records="contacts"
        />
      </PanelBody>
    </Panel>
  );
}

function story(
  start: () => Group,
  options: Omit<Parameters<typeof Editor>[0], "start"> = {},
) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <Editor start={start} {...options} />
      </StoryProviders>
    );
  };
}

// Nothing built yet: describe it, or build it, and nothing else.
export const CalmStart: Story = {
  render: story(() => newGroup("and")),
};

// One condition whose value is still empty.
export const OneIncomplete: Story = {
  render: story(() => newGroup("and", [newLeaf("city", "eq", "")])),
};

// Two conditions joined by "and", the description folded above them.
export const Building: Story = {
  render: story(() =>
    newGroup("and", [
      newLeaf("city", "in", ["Berlin", "Hamburg"]),
      newLeaf("last_activity_at", "lt", { days_ago: 45 }),
    ]),
  ),
};

// An answer that expressed nothing leaves the calm start, and still says what
// it could not use.
export const CouldNotUseOnCalmStart: Story = {
  render: story(() => newGroup("and"), { unused: UNUSED }),
};

// A proposal landed beside the reader's own condition: its rows dashed, and
// the bar above them offering Keep all, Replace my conditions and Undo.
export const ProposalBarOverRows: Story = {
  render: story(() => newGroup("and", [newLeaf("city", "eq", "Berlin")]), {
    landed: {
      text: "in Hamburg, quiet for 45 days",
      proposed: () =>
        newGroup("and", [
          newLeaf("city", "eq", "Hamburg"),
          newLeaf("last_activity_at", "lt", { days_ago: 45 }),
        ]),
    },
  }),
};

// No model is configured: the description gives way to the line saying so,
// and "Add condition" stands beside it as before.
export const NoModel: Story = {
  render: story(() => newGroup("and"), { noModel: true }),
};
