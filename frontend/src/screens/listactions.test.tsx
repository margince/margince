// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { SaveFilterListAction } from "./filterlist";
import { ListScreen } from "./listpage";
import {
  chosenWhy,
  LIVE_ID,
  listsMe,
  liveList,
  MEMBER_ID,
  members,
  SHORTLIST_ID,
  shortlist,
} from "./lists.fixtures";
import { MyViews } from "./myviews";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const empty = { data: [], page: { has_more: false } };

describe("changing a list from its page", () => {
  it("saves a new name, purpose and sharing against the version it opened", async () => {
    const patched: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`GET /lists/${LIVE_ID}/history`]: () => jsonResponse(empty),
      "GET /companies": () => jsonResponse(empty),
      [`PATCH /lists/${LIVE_ID}`]: (body) => {
        patched.push(body);
        return jsonResponse(liveList);
      },
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: en["lists.settings"] }),
    );
    const name = screen.getByRole("textbox", { name: en["lists.name"] });
    await user.clear(name);
    await user.type(name, "Quiet manufacturers");
    await user.clear(
      screen.getByRole("textbox", { name: en["lists.purpose"] }),
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.sharingLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", {
        name: en["lists.sharing.workspace"],
      }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.save"] }));
    await vi.waitFor(() => expect(patched).toHaveLength(1));
    expect(patched[0]).toEqual({
      version: liveList.version,
      name: "Quiet manufacturers",
      purpose: null,
      sharing: "workspace",
    });
  });

  it("archives a list, and a steward takes over one nobody looks after", async () => {
    const calls: string[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(shortlist),
      [`GET /lists/${SHORTLIST_ID}/history`]: () => jsonResponse(empty),
      "GET /companies": () =>
        jsonResponse({
          data: members,
          page: { has_more: true, next_cursor: "c2" },
        }),
      [`DELETE /lists/${SHORTLIST_ID}`]: () => {
        calls.push("archive");
        return jsonResponse(shortlist);
      },
      [`PATCH /lists/${SHORTLIST_ID}`]: (body) => {
        calls.push(`steward:${(body as { steward_id?: string }).steward_id}`);
        return jsonResponse(shortlist);
      },
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <ListScreen listID={SHORTLIST_ID} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", {
        name: en["lists.ownerless.takeOver"],
      }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.archive"] }));
    expect(
      await screen.findByRole("button", { name: en["lists.members.more"] }),
    ).toBeInTheDocument();
    await vi.waitFor(() => expect(calls).toContain("archive"));
    expect(calls.some((c) => c.startsWith("steward:"))).toBe(true);
  });

  it("restores an archived list", async () => {
    let restored = false;
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({ ...liveList, archived_at: "2026-09-25T10:00:00Z" }),
      [`GET /lists/${LIVE_ID}/history`]: () => jsonResponse(empty),
      "GET /companies": () => jsonResponse(empty),
      [`POST /lists/${LIVE_ID}/restore`]: () => {
        restored = true;
        return jsonResponse(liveList);
      },
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: en["lists.restore"] }),
    );
    await vi.waitFor(() => expect(restored).toBe(true));
  });

  it("takes a chosen member off the Shortlist with a note", async () => {
    const removed: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () =>
        jsonResponse({ ...shortlist, health: "ok" }),
      [`GET /lists/${SHORTLIST_ID}/history`]: () => jsonResponse(empty),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${SHORTLIST_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse(chosenWhy),
      [`POST /lists/${SHORTLIST_ID}/members/remove`]: (body) => {
        removed.push(body);
        return new Response(null, { status: 204 });
      },
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <ListScreen listID={SHORTLIST_ID} />
      </StoryProviders>,
    );
    const row = (await screen.findByText("MiTek")).closest("tr") as HTMLElement;
    await user.click(
      within(row).getByRole("button", { name: en["lists.members.why"] }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["lists.remove"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.note"] }),
      "left the company",
    );
    const dialog = screen.getByRole("dialog", {
      name: en["lists.removeTitle"],
    });
    await user.click(
      within(dialog).getByRole("button", { name: en["lists.remove"] }),
    );
    await vi.waitFor(() => expect(removed).toHaveLength(1));
    expect(removed[0]).toEqual({
      entity_type: "company",
      entity_id: MEMBER_ID,
      note: "left the company",
    });
  });
});

describe("saving a filter as a Live List", () => {
  it("saves the complete tree and opens the list", async () => {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      "POST /lists": (body) => {
        posted.push(body);
        return jsonResponse({ ...liveList, id: "saved" }, 201);
      },
    });
    const user = userEvent.setup();
    const tree = newGroup("and", [newLeaf("industry", "eq", "Manufacturing")]);
    render(
      <StoryProviders>
        <SaveFilterListAction resource="company" tree={tree} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: en["filters.saveList"] }),
    );
    await user.type(
      screen.getByRole("textbox", { name: en["lists.name"] }),
      "Manufacturers",
    );
    await user.click(
      screen.getByRole("button", { name: en["filters.saveListConfirm"] }),
    );
    await vi.waitFor(() => expect(window.location.hash).toBe("#/lists/saved"));
    expect(posted[0]).toMatchObject({
      name: "Manufacturers",
      entity_type: "company",
      list_type: "dynamic",
      definition: {
        and: [{ field: "industry", op: "eq", value: "Manufacturing" }],
      },
    });
  });
});

describe("my views", () => {
  it("names the reader's saved filters per record type", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /views": () =>
        jsonResponse({
          data: [
            {
              id: "v1",
              name: "Berlin gold",
              resource: "contacts",
              query: { filter: { field: "city", op: "eq", value: "Berlin" } },
              version: 1,
            },
          ],
          page: { has_more: false },
        }),
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <MyViews />
      </StoryProviders>,
    );
    const row = await screen.findAllByText("Berlin gold");
    await user.click(row[0]);
    expect(window.location.hash).toMatch(/^#\/filters\//);
  });
});
