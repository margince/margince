// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { listsMe, shortlist } from "./lists.fixtures";
import { MyViews } from "./myviews";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const empty = { data: [], page: { has_more: false } };

/** Records the sharing each GET /lists asked for. */
function recordListReads(): string[][] {
  const asked: string[][] = [];
  const inner = globalThis.fetch;
  globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input instanceof Request ? input.url : String(input);
    if (new URL(url, "https://x.local").pathname.endsWith("/lists")) {
      asked.push(
        new URL(url, "https://x.local").searchParams.getAll("sharing"),
      );
    }
    return inner(input, init);
  };
  return asked;
}

function myViews() {
  return render(
    <StoryProviders>
      <MyViews />
    </StoryProviders>,
  );
}

describe("my views", () => {
  it("lists the reader's private lists, and asks for nothing else", async () => {
    const mine = { ...shortlist, name: "Dinner guests", sharing: "private" };
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /views": () => jsonResponse(empty),
      "GET /lists": () =>
        jsonResponse({ data: [mine], page: { has_more: false } }),
    });
    const asked = recordListReads();
    myViews();
    const row = (await screen.findByText("Dinner guests")).closest(
      "tr",
    ) as HTMLElement;
    expect(within(row).getByText(en["lists.kind.shortlist"])).toBeVisible();
    expect(
      screen.getByRole("heading", { name: en["lists.myLists.title"] }),
    ).toBeInTheDocument();
    expect(asked).toEqual([["private"]]);
  });

  it("says where a private list would appear when there is none", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /views": () => jsonResponse(empty),
      "GET /lists": () => jsonResponse(empty),
    });
    myViews();
    expect(
      await screen.findByText(en["lists.myLists.empty"]),
    ).toBeInTheDocument();
  });

  it("starts a Shortlist only the reader can find", async () => {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /views": () => jsonResponse(empty),
      "GET /lists": () => jsonResponse(empty),
      "POST /lists": (body) => {
        posted.push(body);
        return jsonResponse({ ...shortlist, id: "new-list" }, 201);
      },
    });
    const user = userEvent.setup();
    myViews();
    await user.click(
      await screen.findByRole("button", { name: en["lists.newShortlist"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.name"] }),
      "Dinner",
    );
    await user.click(screen.getByRole("button", { name: en["lists.create"] }));
    await vi.waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ sharing: "private" });
  });
});
