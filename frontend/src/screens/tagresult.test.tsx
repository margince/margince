// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { TagResultScreen } from "./tagresult";

// The page a reader lands on from a search hit or a tag pill. Its whole job is
// answering WHICH records carry the word: it once drew three cards reporting
// counts ("1 carry this tag"), which answered how many and named nothing.

const TAG = "01a0641b-db0c-7b92-8564-6c843bf58df3";

function tagRead(usage: { people: number; companies: number; deals: number }) {
  return () =>
    jsonResponse({
      id: TAG,
      name: "Automation World 2026",
      color: "slate",
      version: 1,
      usage,
    });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("the tag page names what carries the word", () => {
  it("lists the records themselves, by name", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 1, companies: 1, deals: 1 }),
      "GET /people": () =>
        jsonResponse({ data: [{ id: "p-1", full_name: "Katrin Hofmann" }] }),
      "GET /organizations": () =>
        jsonResponse({ data: [{ id: "o-1", display_name: "MiTek" }] }),
      "GET /deals": () =>
        jsonResponse({
          data: [{ id: "d-1", name: "netcare GmbH — Einführung" }],
        }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    // The names, which is the whole point — not a count sentence about them.
    expect(await screen.findByText("Katrin Hofmann")).toBeInTheDocument();
    expect(await screen.findByText("MiTek")).toBeInTheDocument();
    expect(
      await screen.findByText("netcare GmbH — Einführung"),
    ).toBeInTheDocument();
  });

  it("opens the record the row names", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 1, companies: 0, deals: 0 }),
      "GET /people": () =>
        jsonResponse({ data: [{ id: "p-1", full_name: "Katrin Hofmann" }] }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    await userEvent.setup().click(await screen.findByText("Katrin Hofmann"));
    expect(window.location.hash).toBe("#/contacts/p-1");
  });

  // A group the word is not on draws nothing. Three cards, two of them
  // reporting zero, is what made the page read as an inventory of absences.
  it("draws no group for a type nothing carries", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 1, companies: 0, deals: 0 }),
      "GET /people": () =>
        jsonResponse({ data: [{ id: "p-1", full_name: "Katrin Hofmann" }] }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    expect(await screen.findByText("Katrin Hofmann")).toBeInTheDocument();
    expect(screen.queryByText(/^Companies/)).toBeNull();
    expect(screen.queryByText(/^Deals/)).toBeNull();
  });

  // A record whose name is empty is NAMED as unnamed rather than drawn as a
  // blank row: a reader cannot press what they cannot see.
  it("names a record carrying no name", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 1, companies: 0, deals: 0 }),
      "GET /people": () =>
        jsonResponse({ data: [{ id: "p-1", full_name: "" }] }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    expect(
      await screen.findByText(en["tagResult.unnamed"]),
    ).toBeInTheDocument();
  });

  // The tag read already said the type carries none, so asking for its rows is
  // a round trip whose answer is known. Two of them, on a tag used on one type.
  it("does not ask for the rows of a type carrying none", async () => {
    let dealCalls = 0;
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 1, companies: 0, deals: 0 }),
      "GET /people": () =>
        jsonResponse({ data: [{ id: "p-1", full_name: "Katrin Hofmann" }] }),
      "GET /deals": () => {
        dealCalls += 1;
        return jsonResponse({ data: [] });
      },
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    expect(await screen.findByText("Katrin Hofmann")).toBeInTheDocument();
    expect(dealCalls).toBe(0);
  });

  // The tag's usage count admits a RETIRED tag; the list filter every record
  // screen uses requires the tag to be live. Heading the group with the usage
  // count therefore promised rows the list would not return.
  it("counts the rows it shows, not the tag's usage", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 3, companies: 0, deals: 0 }),
      "GET /people": () =>
        jsonResponse({ data: [{ id: "p-1", full_name: "Katrin Hofmann" }] }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    expect(await screen.findByText("Katrin Hofmann")).toBeInTheDocument();
    expect(screen.getByText("People (1)")).toBeInTheDocument();
    expect(screen.queryByText("People (3)")).toBeNull();
  });

  // A request that FAILED is not a permission fact. Reporting one as the other
  // tells a reader they may not see records that are simply unfetched.
  it("does not report a failed read as records being withheld", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 2, companies: 0, deals: 0 }),
      "GET /people": () =>
        new Response("nope", {
          status: 500,
          headers: { "content-type": "text/plain" },
        }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    // What the reader sees is the unavailable state, not "nothing carries it".
    // Asserting only the ABSENCE of the empty line would also pass if the group
    // rendered nothing at all, which is how this test first passed for the
    // wrong reason.
    expect(
      await screen.findByText(en["state.unavailable"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(en["tagResult.noneLeft"])).toBeNull();
  });

  // Every record list requires the tag to be LIVE, so a retired word reaches
  // none of them. The header total would be the last line on the page still
  // promising rows the groups correctly do not show.
  it("does not total the assignments of a retired word", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: () =>
        jsonResponse({
          id: TAG,
          name: "Automation World 2026",
          color: "slate",
          version: 2,
          archived_at: "2026-09-01T00:00:00Z",
          usage: { people: 2, companies: 2, deals: 2 },
        }),
      "GET /people": () => jsonResponse({ data: [] }),
      "GET /organizations": () => jsonResponse({ data: [] }),
      "GET /deals": () => jsonResponse({ data: [] }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    expect(
      await screen.findByText("Automation World 2026"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/6 visible assignments/)).toBeNull();
  });

  it("says so when the word is on nothing at all", async () => {
    installFetchStub({
      [`GET /tags/${TAG}`]: tagRead({ people: 0, companies: 0, deals: 0 }),
    });
    render(
      <StoryProviders>
        <TagResultScreen tagID={TAG} />
      </StoryProviders>,
    );

    expect(
      await screen.findByText(en["tagResult.nothingCarries"]),
    ).toBeInTheDocument();
  });
});
