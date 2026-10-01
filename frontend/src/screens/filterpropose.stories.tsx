// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { VocabularyField } from "./filterdata";
import {
  PlainWordsFilter,
  type UnusedPhrase,
  UnusedPhrases,
} from "./filterpropose";
import { type Node, newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Describing a list in plain words. The states worth seeing are the box at rest,
// the question it asks before touching a filter somebody already built, the
// phrases it names back, and the refusal of an installation with no model.
// Press "Propose filter" in each: the answer is canned per story.
const meta: Meta<typeof PlainWordsFilter> = {
  title: "Patterns/Plain-words filter",
  component: PlainWordsFilter,
  parameters: { layout: "padded" },
};
export default meta;

const FIELDS: VocabularyField[] = [
  {
    name: "last_activity_at",
    type: "date",
    operators: ["eq", "neq", "gt", "gte", "lt", "lte", "exists"],
    custom: false,
  },
];

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

function proposalAnswers(status = 200): void {
  installFetchStub({
    "POST /filters/propose": () =>
      status === 200
        ? jsonResponse({
            resource: "company",
            filter: {
              or: [
                {
                  field: "last_activity_at",
                  op: "lt",
                  value: { days_ago: 45 },
                },
                { field: "last_activity_at", op: "exists", value: false },
              ],
            },
            unsupported: UNUSED,
          })
        : jsonResponse(
            { code: "ai_not_configured", status, detail: "no model" },
            status,
          ),
  });
}

function Surface({ start }: Readonly<{ start: Node }>) {
  const [tree, setTree] = useState<Node>(start);
  const [unused, setUnused] = useState<readonly UnusedPhrase[]>([]);
  return (
    <>
      <PlainWordsFilter
        resource="company"
        tree={tree}
        onApply={(next, phrases) => {
          if (next !== null) {
            setTree(next);
          }
          setUnused(phrases);
        }}
      />
      <UnusedPhrases
        unused={unused}
        fields={FIELDS}
        onDismiss={() => setUnused([])}
      />
    </>
  );
}

function story(start: Node, status = 200) {
  return () => {
    proposalAnswers(status);
    return (
      <StoryProviders>
        <Surface start={start} />
      </StoryProviders>
    );
  };
}

type Story = StoryObj<typeof PlainWordsFilter>;

export const OnAnEmptyFilter: Story = {
  // The proposal lands straight in the builder, and the phrases it could not use
  // are listed under it.
  render: story(newGroup("and")),
};

export const OverAFilterAlreadyBuilt: Story = {
  // The reader's own clauses stay until they choose Replace or Add.
  render: story(
    newGroup("and", [newLeaf("last_activity_at", "gte", "2026-01-01")]),
  ),
};

export const WithNoModel: Story = {
  render: story(newGroup("and"), 409),
};

export const PhrasesItCouldNotUse: Story = {
  render: () => (
    <StoryProviders>
      <UnusedPhrases unused={UNUSED} fields={FIELDS} onDismiss={() => {}} />
    </StoryProviders>
  ),
};
