// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import { ImportContextTag, ImportContextTagSummary } from "./import";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The import wizard's two tag surfaces, catalogued apart from the wizard: the
// flow past the first step needs a real file drop, which a story cannot
// perform, so these would otherwise have no rendered state anywhere.

const meta: Meta = {
  title: "Settings/Admin settings/Maintenance/Import/Context tag",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const WORDS = [
  { id: "t-1", workspace_id: "w", name: "K5 Conference", color: "amber" },
  { id: "t-2", workspace_id: "w", name: "January partners", color: "teal" },
];

function withVocabulary(
  words: typeof WORDS,
  truncated: boolean,
  children: React.ReactNode,
) {
  installFetchStub({
    "GET /tags": () =>
      jsonResponse({
        data: words,
        page: { has_more: truncated, next_cursor: null },
      }),
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/** Choosing the word, on the mapping step. Optional, and existing words only. */
export const Picker: Story = {
  render: () =>
    withVocabulary(
      WORDS,
      false,
      <ImportContextTag value="" onChange={() => {}} />,
    ),
};

/**
 * The catalog is capped and carries no cursor, so past the cap a word that
 * exists cannot be picked. The hint says the list is short rather than letting
 * an importer conclude the workspace lacks the word they meant.
 */
export const CatalogCut: Story = {
  render: () =>
    withVocabulary(
      WORDS,
      true,
      <ImportContextTag value="" onChange={() => {}} />,
    ),
};

/**
 * What the approver is committing to. The picker is off screen once a report
 * exists, so without this the chosen word is invisible at the one moment
 * somebody decides whether to write it onto every created record.
 */
export const NamedOnTheReport: Story = {
  render: () =>
    withVocabulary(WORDS, false, <ImportContextTagSummary tagID="t-1" />),
};

/**
 * The vocabulary has not landed, or no longer holds the word. Saying "the tag
 * chosen for this run" is honest; naming the wrong one is not.
 */
export const ChosenButUnnamed: Story = {
  render: () =>
    withVocabulary([], false, <ImportContextTagSummary tagID="t-9" />),
};
