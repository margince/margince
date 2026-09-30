// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { ListScreen } from "./listpage";
import {
  chosenWhy,
  history,
  LIVE_ID,
  listsMe,
  liveList,
  liveWhy,
  MEMBER_ID,
  members,
  SHORTLIST_ID,
  shortlist,
} from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const vocabulary = { resource: "company", fields: [] };

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function page(listID: string) {
  return render(
    <StoryProviders>
      <ListScreen listID={listID} />
    </StoryProviders>,
  );
}

describe("an opened list", () => {
  it("says lists are off rather than asking the server while they are", async () => {
    const fetched: string[] = [];
    installFetchStub({
      "GET /me": listsMe(false),
      [`GET /lists/${LIVE_ID}`]: () => {
        fetched.push("list");
        return jsonResponse(liveList);
      },
    });
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.unavailable"]),
    ).toBeInTheDocument();
    expect(fetched).toEqual([]);
  });

  it("shows a Live List's members and why each is on it, with a hidden value named hidden", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse(liveWhy),
      "GET /filters/vocabulary": () => jsonResponse(vocabulary),
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    expect(await screen.findByText("MiTek")).toBeInTheDocument();
    const row = screen.getByText("MiTek").closest("tr");
    await user.click(
      within(row as HTMLElement).getByRole("button", {
        name: en["lists.members.why"],
      }),
    );
    expect(
      await screen.findByText(en["lists.why.liveMember"]),
    ).toBeInTheDocument();
    expect(screen.getByText(en["lists.why.hidden"])).toBeInTheDocument();
    expect(screen.getByText("Now: Manufacturing")).toBeInTheDocument();
  });

  it("names who chose a Shortlist member and offers a steward to a list nobody looks after", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(shortlist),
      [`GET /lists/${SHORTLIST_ID}/history`]: () =>
        jsonResponse({ data: history, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${SHORTLIST_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse(chosenWhy),
    });
    const user = userEvent.setup();
    page(SHORTLIST_ID);
    expect(
      await screen.findByText(en["lists.ownerless.title"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["lists.ownerless.takeOver"] }),
    ).toBeInTheDocument();
    const row = (await screen.findByText("MiTek")).closest("tr");
    await user.click(
      within(row as HTMLElement).getByRole("button", {
        name: en["lists.members.why"],
      }),
    );
    expect(await screen.findByText(/Chosen by Lena Vogt/)).toBeInTheDocument();
    expect(
      screen.getByText("Signed the quote for the launch deck", {
        selector: "blockquote",
      }),
    ).toBeInTheDocument();
  });

  it("says which retired field a Live List still filters on and who should replace it", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({
          ...liveList,
          health: "retired_field",
          retired_fields: ["cf_last_touch"],
        }),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      "GET /filters/vocabulary": () => jsonResponse(vocabulary),
    });
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.retiredField.title"]),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        en["lists.retiredField.body_one"].replace("{fields}", "last touch"),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getAllByText(en["lists.health.retiredField"]).length,
    ).toBeGreaterThan(0);
  });
});
