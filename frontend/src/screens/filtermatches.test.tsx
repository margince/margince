// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import type { FilterVocabulary } from "./filterdata";
import { FilterOutcome } from "./filtermatches";
import { type Group, newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const VOCABULARY: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "city",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
  ],
};

/** The outcome under a filter the server answers with no match at all. */
function nothingMatches(tree: Group) {
  installFetchStub({
    "POST /filters/preview": () =>
      jsonResponse({
        resource: "contact",
        match_count: 0,
        columns: ["id", "full_name", "city"],
        rows: [],
        truncated: false,
      }),
  });
  render(
    <StoryProviders>
      <FilterOutcome
        tab="contacts"
        tree={tree}
        vocabulary={{ data: VOCABULARY, isPending: false, isError: false }}
      />
    </StoryProviders>,
  );
}

const switchToOr = en["filters.noMatches"].replace("{records}", "contacts");
const loosen = en["filters.noMatchesLoosen"].replace("{records}", "contacts");

describe("a filter nothing matches", () => {
  it("advises an “or” where a group joins conditions with “and”", async () => {
    nothingMatches(
      newGroup("and", [
        newLeaf("city", "eq", "Berlin"),
        newLeaf("city", "eq", "Hamburg"),
      ]),
    );
    expect(await screen.findByText(switchToOr)).toBeInTheDocument();
    expect(screen.queryByText(loosen)).toBeNull();
  });

  it.each([
    ["one condition", newGroup("and", [newLeaf("city", "eq", "Berlin")])],
    [
      "conditions already joined by “or”",
      newGroup("or", [
        newLeaf("city", "eq", "Berlin"),
        newLeaf("city", "eq", "Hamburg"),
      ]),
    ],
  ])("offers no connector to switch under %s", async (_, tree) => {
    nothingMatches(tree);
    expect(await screen.findByText(loosen)).toBeInTheDocument();
    expect(screen.queryByText(switchToOr)).toBeNull();
  });
});
