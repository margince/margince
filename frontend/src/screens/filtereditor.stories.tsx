// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { Panel, PanelBody } from "../design-system/panel";
import type { FilterVocabulary } from "./filterdata";
import { useFilterDraft } from "./filterdraft";
import { FilterEditor } from "./filtereditor";
import type { UnusedPhrase } from "./filterpropose";
import { type Group, newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, StoryProviders } from "./story-utils";

// The editing panel's body. It grows with the filter: two ways in while
// there is no condition, then the rows with the description folded above
// them. What a description could not use is said in both states.
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

function Editor({
  start,
  unused = [],
}: Readonly<{ start: () => Group; unused?: readonly UnusedPhrase[] }>) {
  const [draft, dispatch] = useFilterDraft(start);
  // What an answer with no filter does: the phrases, and the tree untouched.
  useEffect(() => {
    if (unused.length > 0) {
      dispatch({ type: "apply", tree: null, unused });
    }
  }, [dispatch, unused]);
  return (
    <Panel title="Find contacts where…">
      <PanelBody>
        <FilterEditor
          resource="contact"
          draft={draft}
          dispatch={dispatch}
          vocabulary={{ data: VOCABULARY, isPending: false, isError: false }}
        />
      </PanelBody>
    </Panel>
  );
}

function story(start: () => Group, unused?: readonly UnusedPhrase[]) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <Editor start={start} unused={unused} />
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
  render: story(() => newGroup("and"), UNUSED),
};
