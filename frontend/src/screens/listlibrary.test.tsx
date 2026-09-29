// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { ListLibrary } from "./listlibrary";
import { listsMe, liveList, shortlist } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function library() {
  return render(
    <StoryProviders>
      <ListLibrary />
    </StoryProviders>,
  );
}

describe("the team's lists", () => {
  it("names each list with its kind, count, steward and a steward that is missing", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({
          data: [liveList, shortlist],
          page: { has_more: false },
        }),
    });
    library();
    const live = (await screen.findByText(liveList.name)).closest(
      "tr",
    ) as HTMLElement;
    expect(within(live).getByText(en["lists.kind.live"])).toBeInTheDocument();
    expect(within(live).getByText("42")).toBeInTheDocument();
    expect(within(live).getByText("Lena Vogt")).toBeInTheDocument();
    const chosen = screen
      .getByText(shortlist.name)
      .closest("tr") as HTMLElement;
    expect(
      within(chosen).getByText(en["lists.health.ownerless"]),
    ).toBeInTheDocument();
    expect(within(chosen).getByText(en["lists.noSteward"])).toBeInTheDocument();
  });

  it("asks the server again when the reader searches or narrows by kind", async () => {
    const asked: string[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () => jsonResponse({ data: [], page: { has_more: false } }),
    });
    const inner = globalThis.fetch;
    globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/lists")) {
        asked.push(new URL(url, "https://x.local").search);
      }
      return inner(input, init);
    };
    const user = userEvent.setup();
    library();
    expect(
      await screen.findByText(en["lists.library.empty"]),
    ).toBeInTheDocument();
    await user.type(screen.getByRole("searchbox"), "K5");
    await user.click(
      screen.getByRole("button", { name: en["lists.kind.shortlist"] }),
    );
    await vi.waitFor(() =>
      expect(
        asked.some((q) => q.includes("q=K5") && q.includes("list_type=static")),
      ).toBe(true),
    );
  });

  it("starts a Shortlist of the chosen record type and opens it", async () => {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () => jsonResponse({ data: [], page: { has_more: false } }),
      "POST /lists": (body) => {
        posted.push(body);
        return jsonResponse({ ...shortlist, id: "new-list" }, 201);
      },
    });
    const user = userEvent.setup();
    library();
    await user.click(
      await screen.findByRole("button", { name: en["lists.newShortlist"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.name"] }),
      "Dinner",
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.recordTypeLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: en["lists.type.company"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.purpose"] }),
      "October",
    );
    await user.click(screen.getByRole("button", { name: en["lists.create"] }));
    await vi.waitFor(() =>
      expect(window.location.hash).toBe("#/lists/new-list"),
    );
    expect(posted[0]).toEqual({
      name: "Dinner",
      entity_type: "company",
      list_type: "static",
      purpose: "October",
      sharing: "team",
    });
  });
});
