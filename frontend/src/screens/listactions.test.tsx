// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { ListScreen } from "./listpage";
import {
  LIVE_ID,
  listsMe,
  liveList,
  members,
  SHORTLIST_ID,
  shortlist,
  TEAM_ID,
  teamsPage,
} from "./lists.fixtures";
import { ListSettingsAction } from "./listsettings";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const empty = { data: [], page: { has_more: false } };

// Edit list and Archive list sit behind the list's own ⋯ menu.
const moreFor = (name: string) =>
  en["filters.library.rowMore"].replace("{name}", name);

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
      await screen.findByRole("button", { name: moreFor(liveList.name) }),
    );
    await user.click(
      screen.getByRole("button", { name: en["lists.settings"] }),
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

  it("moves a team list to one named team", async () => {
    const patched: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true, [TEAM_ID]),
      "GET /teams": () => jsonResponse(teamsPage),
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
      await screen.findByRole("button", { name: moreFor(liveList.name) }),
    );
    await user.click(
      screen.getByRole("button", { name: en["lists.settings"] }),
    );
    await user.click(
      screen.getByRole("combobox", { name: en["lists.teamLabel"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: "Team Germany" }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.save"] }));
    await vi.waitFor(() => expect(patched).toHaveLength(1));
    expect(patched[0]).toMatchObject({ team_id: TEAM_ID });
    expect(patched[0]).not.toHaveProperty("sharing");
  });

  it("leaves the audience alone when a refetch moves it while the form is open", async () => {
    const patched: unknown[] = [];
    installFetchStub({
      "GET /me": listsMe(true, [TEAM_ID]),
      "GET /teams": () => jsonResponse(teamsPage),
      [`PATCH /lists/${LIVE_ID}`]: (body) => {
        patched.push(body);
        return jsonResponse(liveList);
      },
    });
    const user = userEvent.setup();
    const { rerender } = render(
      <StoryProviders>
        <ListSettingsAction list={liveList} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: en["lists.settings"] }),
    );
    // Somebody else narrowed the list to one team; the page refetched it.
    rerender(
      <StoryProviders>
        <ListSettingsAction
          list={{
            ...liveList,
            team_id: TEAM_ID,
            sharing: "team",
            version: liveList.version + 1,
          }}
        />
      </StoryProviders>,
    );
    const name = screen.getByRole("textbox", { name: en["lists.name"] });
    await user.clear(name);
    await user.type(name, "Renamed");
    await user.click(screen.getByRole("button", { name: en["lists.save"] }));
    await vi.waitFor(() => expect(patched).toHaveLength(1));
    expect(patched[0]).toEqual({
      version: liveList.version,
      name: "Renamed",
      purpose: liveList.purpose,
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
    await user.click(
      screen.getByRole("button", { name: moreFor(shortlist.name) }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.archive"] }));
    expect(await screen.findByText("MiTek")).toBeInTheDocument();
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
});
