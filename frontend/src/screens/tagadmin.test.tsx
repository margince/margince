// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { TagVocabularyCard } from "./tagadmin";

// Settings › Data model: the one door that coins a word. Every verb but merge
// is reversible, and merge says so.

const KEY_ACCOUNT = {
  id: "t-1",
  workspace_id: "w",
  name: "Key Account",
  color: "amber",
  version: 3,
};
const RETIRED = {
  id: "t-2",
  workspace_id: "w",
  name: "Trade Fair 2025",
  version: 1,
  archived_at: "2026-01-01T00:00:00Z",
};

const ADMIN = { tag: ["read", "create", "update", "delete"] };

function mount(
  words: readonly unknown[],
  grants: Record<string, string[]> = ADMIN,
  extra: Record<string, () => Response> = {},
) {
  installFetchStub({
    "GET /me": meRoute(grants as never),
    "GET /tags": () =>
      jsonResponse({
        data: words,
        page: { has_more: false, next_cursor: null },
      }),
    "GET /tags/t-1": () =>
      jsonResponse({
        ...KEY_ACCOUNT,
        usage: { people: 4, companies: 2, deals: 1 },
      }),
    "GET /tags/t-2": () =>
      jsonResponse({
        ...RETIRED,
        usage: { people: 0, companies: 0, deals: 0 },
      }),
    ...extra,
  });
  render(
    <StoryProviders>
      <TagVocabularyCard />
    </StoryProviders>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the tag vocabulary card", () => {
  it("draws the workspace's words with how much carries them", async () => {
    mount([KEY_ACCOUNT]);
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
    // 4 people + 2 companies + 1 deal, counted within what this reader sees.
    await waitFor(() =>
      expect(screen.getByText(/7 records/)).toBeInTheDocument(),
    );
  });

  // A retired word is restored HERE, so a list that hid it would leave the
  // verb with nothing to act on and an admin concluding the word was deleted.
  it("lists a retired word, offering to restore rather than retire it", async () => {
    mount([RETIRED]);
    expect(await screen.findByText(/Trade Fair 2025/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["tagAdmin.restore"] }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["tagAdmin.archive"] }),
    ).toBeNull();
  });

  // Applying a tag is every seat's; coining one is not. The card is drawn for
  // a reader who may see the vocabulary, and the verbs answer to the grants.
  it("offers no verbs to a seat that may only read the vocabulary", async () => {
    mount([KEY_ACCOUNT], { tag: ["read"] });
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["tagAdmin.add"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["tagAdmin.edit"] }),
    ).toBeNull();
  });

  // The accident this prevents: an admin looks for a word, does not see it,
  // and is one press from a second spelling of one that already exists.
  it("warns when a new name is close to a word the workspace has", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT]);
    await user.click(
      await screen.findByRole("button", { name: en["tagAdmin.add"] }),
    );
    await user.type(
      screen.getByLabelText(en["tagAdmin.nameLabel"]),
      "key-account",
    );
    expect(
      await screen.findByText(/Close to a word this organization already has/),
    ).toBeInTheDocument();
  });

  // Merge is the one verb that cannot be undone, and the released name is the
  // part an admin does not expect.
  it("says merge cannot be undone and releases the name", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT, { ...RETIRED, archived_at: null }]);
    // The row's OWN verb: both live words offer one, and pressing "the first
    // Merge button" would be a test that passes whichever row it opened.
    const row = (await screen.findByText("Key Account")).closest("li");
    expect(row).not.toBeNull();
    await user.click(
      within(row as HTMLElement).getByRole("button", {
        name: en["tagAdmin.merge"],
      }),
    );
    const warning = await screen.findByText(/This cannot be undone/);
    expect(warning).toBeInTheDocument();
    expect(warning.textContent).toContain("released");
  });
});
