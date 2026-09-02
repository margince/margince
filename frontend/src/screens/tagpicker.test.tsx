// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { AddTagDialog } from "./tagpicker";

// The catalog is capped and carries no cursor, so a workspace past the cap gets
// a CUT list. A missing word then reads as a word the workspace does not have,
// and the reader asks an admin to coin the duplicate this dialog prevents.

const ORG = "01a06151-0000-7000-8000-000000000001";

function mount(truncated: boolean) {
  installFetchStub({
    "GET /tags": () =>
      jsonResponse({
        data: [{ id: "t-1", workspace_id: "w", name: "Key Account" }],
        page: { has_more: truncated, next_cursor: null },
      }),
  });
  render(
    <StoryProviders>
      <AddTagDialog
        entityType="organization"
        entityID={ORG}
        current={[]}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the add-tag dialog", () => {
  // The control: a notice that never draws would pass the case below for the
  // wrong reason.
  it("says nothing about length when the whole catalog fits", async () => {
    mount(false);
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
    expect(screen.queryByText(en["tags.catalogTruncated"])).toBeNull();
  });

  it("says the list is cut when the catalog was truncated", async () => {
    mount(true);
    expect(
      await screen.findByText(en["tags.catalogTruncated"]),
    ).toBeInTheDocument();
  });
});
