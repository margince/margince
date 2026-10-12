// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";

import { ToastProvider, ToastRegion } from "../design-system/toast";

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
  title: "Settings/Sales/Tags/Tag vocabulary",
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
    carried_by: 60,
  },
  {
    id: "t-2",
    workspace_id: "w",
    name: "Churn Risk",
    color: "rose",
    version: 1,
    carried_by: 14,
  },
  {
    id: "t-3",
    workspace_id: "w",
    name: "Trade Fair 2025",
    version: 1,
    archived_at: "2026-01-01T00:00:00Z",
    carried_by: 1,
  },
  {
    id: "t-4",
    workspace_id: "w",
    name: "Partner programme for regional resellers in the DACH market",
    color: "sky",
    version: 2,
    carried_by: 1280,
  },
];

function Card({
  words = WORDS,
  grants = { tag: ["read", "create", "update", "delete"] },
}: Readonly<{ words?: typeof WORDS; grants?: Record<string, string[]> }>) {
  installFetchStub({
    "GET /me": meRoute(grants as never),
    "POST /tags/t-2/merge": () => jsonResponse({ moved: 9, collapsed: 2 }),
    "DELETE /tags/t-2": () => new Response(null, { status: 204 }),
    "POST /tags/t-2/restore": () => new Response(null, { status: 204 }),
    "GET /tags": () =>
      jsonResponse({
        data: words,
        page: { has_more: false, next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <ToastProvider>
        <TagVocabularyCard />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

// The row's own menu, so a play cannot pass by opening another row's.
async function pressRowVerb(canvasElement: HTMLElement, verb: string) {
  const body = within(canvasElement.ownerDocument.body);
  await userEvent.click(
    await body.findByRole("button", { name: "Actions for Churn Risk" }),
  );
  await userEvent.click(await body.findByRole("button", { name: verb }));
  return body;
}

/** The vocabulary as an admin meets it: live words, a retired one, and how
 * much of the workspace carries each. */
export const Governed: Story = {
  render: () => <Card />,
};

/** The same table in dark: the row hairlines and the hover ground are tokens. */
export const GovernedDark: Story = {
  globals: { theme: "dark" },
  render: () => <Card />,
};

/** A row's verbs, in the one menu at its end. */
export const RowMenu: Story = {
  render: () => <Card />,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Actions for Churn Risk" }),
    );
    await expect(
      await body.findByRole("button", { name: "Merge" }),
    ).toBeVisible();
  },
};

/** Retire runs at once: no dialog, and the toast's Undo is the way back. */
export const RetireOffersUndo: Story = {
  render: () => <Card />,
  play: async ({ canvasElement }) => {
    const body = await pressRowVerb(canvasElement, "Retire");
    const undo = await body.findByRole("button", { name: "Undo" });
    await waitFor(() => expect(undo).toBeVisible());
    await expect(body.queryByRole("dialog")).not.toBeInTheDocument();
  },
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

/**
 * The catalog is capped and carries no cursor. An admin shown a cut list would
 * coin a duplicate of a word past the cap, and merge could not name it.
 */
export const CatalogCut: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({
        tag: ["read", "create", "update", "delete"],
      } as never),
      "GET /tags": () =>
        jsonResponse({
          data: WORDS,
          page: { has_more: true, next_cursor: null },
        }),
    });
    return (
      <StoryProviders>
        <TagVocabularyCard />
      </StoryProviders>
    );
  },
};

/**
 * A seat the settings entry opened for on another grant entirely — the tab
 * unions five data-model reads. The card says the vocabulary is withheld
 * rather than drawing an empty list, which would read as "no tags here".
 */
export const VocabularyWithheld: Story = {
  render: () => <Card grants={{ custom_field: ["read"] }} />,
};

/**
 * At a phone width. Each row folds onto two lines: the word and its menu, then
 * how many records carry it.
 */
export const Narrow: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <div style={{ maxWidth: "22rem" }}>
      <Card />
    </div>
  ),
};

/** After a merge: what moved and what collapsed, counted apart. */
export const Merged: Story = {
  render: () => <Card />,
  play: async ({ canvasElement }) => {
    const body = await pressRowVerb(canvasElement, "Merge");
    await userEvent.click(
      await body.findByRole("combobox", { name: "Keep this tag" }),
    );
    await userEvent.click(
      await body.findByRole("option", { name: "Key Account" }),
    );
    const dialog = within(await body.findByRole("dialog"));
    await userEvent.click(dialog.getByRole("button", { name: "Merge" }));
    await body.findByRole("heading", { name: "Merged" });
  },
};

/** Editing a word: its name and its colour, committed together. */
export const EditingTag: Story = {
  render: () => <Card />,
  play: async ({ canvasElement }) => {
    const body = await pressRowVerb(canvasElement, "Edit");
    await body.findByRole("dialog", { name: "Edit tag" });
  },
};
