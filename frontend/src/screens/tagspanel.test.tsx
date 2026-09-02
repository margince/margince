// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { TagsPanel } from "./tagspanel";

// What the panel SAYS in each state. Three of them answer 200 with a list, so
// the wire cannot tell them apart — only the words on screen can, which is
// what these assert.

const ORG = "01a06151-0000-7000-8000-000000000001";

type PanelTag = {
  tag_id: string;
  name: string;
  color?: string;
  archived: boolean;
  assigned_at: string;
  assigned_by?: { display_name: string; kind: string };
};

function mount(tags: PanelTag[], withheld = false, canEdit = true) {
  installFetchStub({
    [`GET /records/organization/${ORG}/tags`]: () =>
      jsonResponse({ data: tags, withheld }),
  });
  render(
    <StoryProviders>
      <TagsPanel entityType="organization" entityID={ORG} canEdit={canEdit} />
    </StoryProviders>,
  );
}

const KEY_ACCOUNT: PanelTag = {
  tag_id: "t-1",
  name: "Key Account",
  color: "amber",
  archived: false,
  assigned_at: "2026-03-03T10:00:00Z",
  assigned_by: { display_name: "Lena Fischer", kind: "human" },
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the tags panel", () => {
  // A malformed answer must not take the RECORD PAGE down with it. This one
  // is what a stub or a server disagreeing with the contract sends, and
  // reading .slice off the missing list crashed every company screen test
  // until the default landed.
  it("survives an answer that carries no list at all", async () => {
    installFetchStub({
      [`GET /records/organization/${ORG}/tags`]: () => jsonResponse({}),
    });
    render(
      <StoryProviders>
        <TagsPanel entityType="organization" entityID={ORG} canEdit />
      </StoryProviders>,
    );
    // The panel draws its empty state rather than throwing.
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
  });

  it("draws the words a record carries", async () => {
    mount([KEY_ACCOUNT]);
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
  });

  // The distinction the whole read exists to carry: a caller who may see the
  // record and not the vocabulary must not be told the record has no tags.
  it("says the words are withheld rather than claiming there are none", async () => {
    mount([], true);
    expect(await screen.findByText(en["tags.withheld"])).toBeInTheDocument();
    expect(screen.queryByText(en["tags.emptyTitle"])).toBeNull();
  });

  it("teaches what tags are for when a record carries none", async () => {
    mount([]);
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByText(en["tags.withheld"])).toBeNull();
  });

  // Past four the rest fold away, and the reader can open them.
  it("folds the words past the fourth behind an expander", async () => {
    const user = userEvent.setup();
    mount(
      ["A", "B", "C", "D", "E", "F"].map((name, i) => ({
        ...KEY_ACCOUNT,
        tag_id: `t-${i}`,
        name,
      })),
    );
    expect(await screen.findByText("A")).toBeInTheDocument();
    expect(screen.queryByText("F")).toBeNull();

    await user.click(screen.getByRole("button", { name: /\+2/ }));
    await waitFor(() => expect(screen.getByText("F")).toBeInTheDocument());
  });

  // Applying a tag writes to the RECORD, so a reader who may only look at one
  // sees the words and no verb to change them.
  it("offers no remove verb to a reader who may not edit the record", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT], false, false);
    await screen.findByText("Key Account");

    await user.click(
      screen.getByRole("button", {
        name: en["tags.options"].replace("{name}", "Key Account"),
      }),
    );
    expect(screen.queryByText(en["tags.removeFromRecord"])).toBeNull();
  });

  it("names who applied a tag, and when", async () => {
    const user = userEvent.setup();
    mount([KEY_ACCOUNT]);
    await screen.findByText("Key Account");

    await user.click(
      screen.getByRole("button", {
        name: en["tags.options"].replace("{name}", "Key Account"),
      }),
    );
    expect(await screen.findByText(/Lena Fischer/)).toBeInTheDocument();
    // The workspace-visibility line rides with it: a reader deciding whether
    // to tag a sensitive record has to know the word is not private to them.
    expect(
      screen.getByText(en["tags.visibleWorkspaceWide"]),
    ).toBeInTheDocument();
  });

  // An assignment written before the product recorded WHO has nobody to
  // credit, and inventing a name would put a choice on somebody.
  it("shows the date alone when the assignment names nobody", async () => {
    const user = userEvent.setup();
    mount([{ ...KEY_ACCOUNT, assigned_by: undefined }]);
    await screen.findByText("Key Account");

    await user.click(
      screen.getByRole("button", {
        name: en["tags.options"].replace("{name}", "Key Account"),
      }),
    );
    expect(screen.queryByText(/Lena Fischer/)).toBeNull();
    expect(
      screen.getByText(en["tags.visibleWorkspaceWide"]),
    ).toBeInTheDocument();
  });
});
