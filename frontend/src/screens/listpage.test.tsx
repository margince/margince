// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { ListScreen } from "./listpage";
import {
  chosenWhy,
  history,
  LIVE_ID,
  listsMe,
  liveHistory,
  liveList,
  liveWhy,
  MEMBER_ID,
  members,
  SHORTLIST_ID,
  shortlist,
  visitAnswer,
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

  it("records the visit once the list is read, reads it again, and marks the members the server says joined", async () => {
    const visits: string[] = [];
    let listReads = 0;
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => {
        listReads++;
        return jsonResponse(liveList);
      },
      [`POST /lists/${LIVE_ID}/visit`]: () => {
        visits.push(listReads > 0 ? "after the read" : "before the read");
        return visitAnswer(LIVE_ID)();
      },
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: liveHistory, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    page(LIVE_ID);
    const joined = (await screen.findByText("MiTek")).closest(
      "tr",
    ) as HTMLElement;
    expect(
      await within(joined).findByText(en["lists.members.new"]),
    ).toBeInTheDocument();
    const stayed = screen.getByText("Nordfracht").closest("tr") as HTMLElement;
    expect(
      within(stayed).queryByText(en["lists.members.new"]),
    ).not.toBeInTheDocument();
    expect(visits).toEqual(["after the read"]);
    await waitFor(() => expect(listReads).toBe(2));
  });

  it("says when a Live List was last checked and what it gained and lost since the last visit", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: liveHistory, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    page(LIVE_ID);
    expect(await screen.findByText(/^Last checked /)).toBeInTheDocument();
    expect(
      screen.getByText("Since your last visit: 3 joined, 1 left"),
    ).toBeInTheDocument();
  });

  it("says a Live List not checked yet records changes from its first check", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({
          ...liveList,
          last_check: undefined,
          since_last_visit: undefined,
        }),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.head.notChecked"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/^Since your last visit/),
    ).not.toBeInTheDocument();
  });

  it("names a Live List's observed changes as of the check that saw them, and says what a check cannot see", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: liveHistory, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    page(LIVE_ID);
    expect(
      await screen.findByText(/^Joined as of .+ · after the filter changed$/),
    ).toBeInTheDocument();
    expect(screen.getByText(/^Left as of [^·]+$/)).toBeInTheDocument();
    expect(screen.getAllByText(en["lists.history.checker"])).toHaveLength(2);
    expect(screen.getByText(en["lists.history.liveNote"])).toBeInTheDocument();
  });

  it("keeps the check note off a Shortlist's history", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(shortlist),
      [`POST /lists/${SHORTLIST_ID}/visit`]: visitAnswer(SHORTLIST_ID),
      [`GET /lists/${SHORTLIST_ID}/history`]: () =>
        jsonResponse({ data: history, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    page(SHORTLIST_ID);
    expect(
      await screen.findByText(/^Added a record · by hand/),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(en["lists.history.liveNote"]),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/^Last checked /)).not.toBeInTheDocument();
  });

  it("says what changed since the last visit in one sentence whose records open", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({
          ...liveList,
          changes_since_visit: {
            since: "2026-09-28T17:00:00Z",
            joined: {
              count: 4,
              records: [
                { entity_id: MEMBER_ID, name: "Acme" },
                { entity_id: SHORTLIST_ID, name: "Globex" },
              ],
            },
            left: {
              count: 1,
              records: [{ entity_id: LIVE_ID, name: "Initech" }],
            },
            filter_changes: 1,
          },
        }),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    page(LIVE_ID);
    const acme = await screen.findByRole("button", { name: "Acme" });
    const sentence = acme.closest("p");
    expect(sentence).toHaveTextContent(
      /^Since your visit on .+: 4 joined \(Acme, Globex, \+2 more\), 1 left \(Initech\)\. The filter changed once\.$/,
    );
    expect(
      within(sentence as HTMLElement).getByRole("button", { name: "Initech" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Since your last visit: 3 joined, 1 left"),
    ).not.toBeInTheDocument();
  });

  it("names the automations an archive will pause before archiving", async () => {
    const archived: string[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({
          ...liveList,
          dependencies: [
            {
              kind: "automation",
              occurred_at: "2026-09-20T08:00:00Z",
              blocking: false,
              role: "watches",
              automation_id: MEMBER_ID,
              automation_name: "Follow up on quiet manufacturers",
            },
          ],
        }),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`DELETE /lists/${LIVE_ID}`]: () => {
        archived.push(LIVE_ID);
        return jsonResponse({
          ...liveList,
          archived_at: "2026-09-30T08:00:00Z",
        });
      },
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    await user.click(
      await screen.findByRole("button", { name: en["lists.archive"] }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(
        "Follow up on quiet manufacturers watches this list",
      ),
    ).toBeInTheDocument();
    expect(archived).toEqual([]);
    await user.click(
      within(dialog).getByRole("button", { name: en["lists.archive"] }),
    );
    await waitFor(() => expect(archived).toEqual([LIVE_ID]));
    expect(screen.queryByText(/^Exported /)).not.toBeInTheDocument();
  });
});
