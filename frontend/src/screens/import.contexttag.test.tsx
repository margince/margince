// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { ImportContextTag, ImportContextTagSummary } from "./import";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The picker sits on the mapping step, which is off screen once a report
// exists — so the run's word has to be named again beside the outcome, at the
// one moment somebody decides whether to commit it.

const WORDS = [
  { id: "t-1", workspace_id: "w", name: "K5 Conference", color: "amber" },
];

function mount(node: React.ReactNode, words = WORDS, truncated = false) {
  installFetchStub({
    "GET /tags": () =>
      jsonResponse({
        data: words,
        page: { has_more: truncated, next_cursor: null },
      }),
  });
  render(<StoryProviders>{node}</StoryProviders>);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the import's context tag", () => {
  it("names the chosen word on the report", async () => {
    mount(<ImportContextTagSummary tagID="t-1" />);
    expect(await screen.findByText(/K5 Conference/)).toBeInTheDocument();
  });

  // Naming the wrong word, or none, would be worse than saying "the tag chosen
  // for this run" — the approver is about to write it onto every created row.
  it("says a word was chosen without naming one it cannot resolve", async () => {
    mount(<ImportContextTagSummary tagID="t-9" />, []);
    expect(
      await screen.findByText(en["import.contextTagChosenUnnamed"]),
    ).toBeInTheDocument();
  });

  it("says nothing when the run is filed under no word", () => {
    mount(<ImportContextTagSummary tagID="" />);
    expect(screen.queryByText(/filed under/)).toBeNull();
  });

  // Past the cap a word that exists cannot be picked, and an importer who
  // cannot find the word they meant asks an admin to coin a duplicate.
  it("says the list is short when the catalog was cut", async () => {
    mount(<ImportContextTag value="" onChange={() => {}} />, WORDS, true);
    expect(
      await screen.findByText(new RegExp(en["tags.catalogTruncated"])),
    ).toBeInTheDocument();
  });
});
