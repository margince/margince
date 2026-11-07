// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import type { FilterVocabulary } from "../../lib/filterdata";
import { type Group, newGroup, newLeaf } from "../../lib/segmentpredicate";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { FilterOutcome } from "./filtermatches";

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

describe("the results head", () => {
  it("shows Export beside the count without opening a menu", async () => {
    nothingMatches(newGroup("and", [newLeaf("city", "eq", "Berlin")]));
    const exportCsv = await screen.findByRole("button", {
      name: en["filters.exportCsv"],
    });
    const head = screen
      .getByRole("heading", { name: en["filters.resultsTitle"] })
      .closest("header");

    expect(head?.contains(exportCsv)).toBe(true);
    expect(
      head?.contains(
        screen.getByRole("button", { name: en["filters.exportJson"] }),
      ),
    ).toBe(true);
  });
});

describe("export while the filter changes", () => {
  it("waits for the new filter's rows before it offers the file", async () => {
    const answers: Array<() => void> = [];
    installFetchStub({
      "POST /filters/preview": () =>
        new Promise<Response>((resolve) => {
          answers.push(() =>
            resolve(
              jsonResponse({
                resource: "contact",
                match_count: 1,
                columns: ["id", "full_name", "city"],
                rows: [{ id: "c1", full_name: "Ann Lee", city: "Berlin" }],
                truncated: false,
              }),
            ),
          );
        }),
    });
    const outcome = (tree: Group) => (
      <StoryProviders>
        <FilterOutcome
          tab="contacts"
          tree={tree}
          vocabulary={{ data: VOCABULARY, isPending: false, isError: false }}
        />
      </StoryProviders>
    );
    const { rerender } = render(
      outcome(newGroup("and", [newLeaf("city", "eq", "Berlin")])),
    );
    await waitFor(() => expect(answers).toHaveLength(1));
    answers[0]?.();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["filters.exportCsv"] }),
      ).toBeEnabled(),
    );

    rerender(outcome(newGroup("and", [newLeaf("city", "eq", "Hamburg")])));
    await waitFor(() => expect(answers).toHaveLength(2));
    expect(
      screen.getByRole("button", { name: en["filters.exportCsv"] }),
    ).toBeDisabled();

    answers[1]?.();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["filters.exportCsv"] }),
      ).toBeEnabled(),
    );
  });
});
