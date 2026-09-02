// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { TagVocabularyCard } from "./tagadmin";

// Settings › Data model. The one door that coins a word: applying an existing
// tag is every seat's, and this card is Admin's and Ops's, because a vocabulary
// anybody may extend is a list of everything anybody ever typed.

const meta: Meta = {
  title: "Settings/Tag vocabulary",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const WORDS = [
  {
    id: "t-1",
    workspace_id: "w",
    name: "Key Account",
    color: "amber",
    version: 3,
  },
  {
    id: "t-2",
    workspace_id: "w",
    name: "Churn Risk",
    color: "rose",
    version: 1,
  },
  {
    id: "t-3",
    workspace_id: "w",
    name: "Trade Fair 2025",
    version: 1,
    archived_at: "2026-01-01T00:00:00Z",
  },
];

const USAGE: Record<
  string,
  { people: number; companies: number; deals: number }
> = {
  "t-1": { people: 41, companies: 12, deals: 7 },
  "t-2": { people: 3, companies: 9, deals: 2 },
  "t-3": { people: 0, companies: 0, deals: 0 },
};

function Card({
  words = WORDS,
  grants = { tag: ["read", "create", "update", "delete"] },
}: Readonly<{ words?: typeof WORDS; grants?: Record<string, string[]> }>) {
  installFetchStub({
    "GET /me": meRoute(grants as never),
    "GET /tags": () =>
      jsonResponse({
        data: words,
        page: { has_more: false, next_cursor: null },
      }),
    ...Object.fromEntries(
      words.map((word) => [
        `GET /tags/${word.id}`,
        () => jsonResponse({ ...word, usage: USAGE[word.id] }),
      ]),
    ),
  });
  return (
    <StoryProviders>
      <TagVocabularyCard />
    </StoryProviders>
  );
}

/** The vocabulary as an admin meets it: live words, a retired one, and how
 * much of the workspace carries each. */
export const Governed: Story = {
  render: () => <Card />,
};

/**
 * A seat that may see the vocabulary and not change it. The words are drawn
 * and the verbs are not: the server is the authority, and a control whose only
 * outcome is a refusal is worse than no control.
 */
export const ReadOnly: Story = {
  render: () => <Card grants={{ tag: ["read"] }} />,
};

/** Before the first word. It says what the feature is for rather than
 * reporting that a list is empty. */
export const Empty: Story = {
  render: () => <Card words={[]} />,
};
