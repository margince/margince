// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
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

describe("a filter over an extension's kind", () => {
  it("heads each column with the label the kind declares", async () => {
    installFetchStub({
      "GET /record-kinds": () =>
        jsonResponse({
          data: [
            {
              name: "ext:hotel:room",
              unit: "hotel",
              kind: "room",
              display_name: "Room",
              fields: [{ name: "room_no", label: "Room number" }],
              natural_key: ["room_no"],
              delete_policy: "archive_only",
              actions: ["read"],
            },
          ],
        }),
      "POST /filters/preview": () =>
        jsonResponse({
          resource: "ext:hotel:room",
          match_count: 1,
          columns: ["id", "room_no", "floor"],
          rows: [{ id: "r1", room_no: "101", floor: "1" }],
          truncated: false,
        }),
    });
    render(
      <StoryProviders>
        <FilterOutcome
          tab="ext:hotel:room"
          tree={newGroup("and", [newLeaf("room_no", "eq", "101")])}
          vocabulary={{
            data: { resource: "ext:hotel:room", fields: [] },
            isPending: false,
            isError: false,
          }}
        />
      </StoryProviders>,
    );
    expect(
      await screen.findByRole("columnheader", { name: "Room number" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "floor" }),
    ).toBeInTheDocument();
    expect(screen.getByText("1 Room record matches")).toBeInTheDocument();
  });
});

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
