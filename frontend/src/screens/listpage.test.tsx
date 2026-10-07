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

import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { en } from "../i18n/en";
import { ListScreen } from "./listpage";
import {
  chosenListing,
  exportDependencies,
  history,
  LIVE_ID,
  listingAnswer,
  listsMe,
  liveHistory,
  liveList,
  liveListing,
  liveVocabulary,
  MEMBER_ID,
  members,
  SHORTLIST_ID,
  shortlist,
  visitAnswer,
} from "./lists.fixtures";
import type { List } from "./lists.queries";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

const vocabulary = { resource: "company", fields: [] };

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(URL, "createObjectURL");
  Reflect.deleteProperty(URL, "revokeObjectURL");
});

function page(listID: string) {
  return render(
    <StoryProviders>
      <ListScreen listID={listID} />
    </StoryProviders>,
  );
}

/** Every answer a Live List's page reads, `list` standing for the list. */
function stubLivePage(list: List, routes: RouteMap = {}) {
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /lists/${LIVE_ID}`]: () => jsonResponse(list),
    [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
    [`GET /lists/${LIVE_ID}/history`]: () => jsonResponse(emptyPage),
    "GET /companies": () =>
      jsonResponse({ data: members, page: { has_more: false } }),
    ...routes,
  });
}

const moreFor = (name: string) =>
  en["filters.library.rowMore"].replace("{name}", name);

function whatChanged() {
  return screen.findByRole("region", { name: en["lists.history.title"] });
}

/** The facts under the list's name, found by the label every list carries. */
function facts(): HTMLElement {
  const strip = screen.getByText(en["lists.col.recordType"]).closest("dl");
  if (!(strip instanceof HTMLElement)) {
    throw new Error("the Records label sits in no facts list");
  }
  return strip;
}

/** The panel a ⋯ trigger opens, which is portalled away from the trigger. */
function menuOf(trigger: HTMLElement): HTMLElement {
  const menu = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  if (menu === null) {
    throw new Error("the menu trigger controls no panel");
  }
  return menu;
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
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(
      screen.getByRole("heading", { level: 1, name: en["lists.page"] }),
    ).toBeInTheDocument();
  });

  // The page heads itself, so every state before it has a list to name still
  // prints the one heading a reader navigating by heading lands on.
  it("prints one heading while the list is being read", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => new Promise<Response>(() => {}),
    });
    page(LIVE_ID);
    expect(await screen.findByRole("status")).toHaveTextContent(
      en["lists.loading"],
    );
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(
      screen.getByRole("heading", { level: 1, name: en["lists.page"] }),
    ).toBeInTheDocument();
  });

  it("prints one heading when the list is gone", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({ title: "Not found", status: 404 }, 404),
    });
    page(LIVE_ID);
    expect(await screen.findByText(en["lists.gone"])).toBeInTheDocument();
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(
      screen.getByRole("heading", { level: 1, name: en["lists.page"] }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["filters.backToLibrary"] }),
    ).toBeInTheDocument();
  });

  it("shows each Live List member's filter fields as columns, a hidden value named hidden, and no Why", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${LIVE_ID}/members`]: listingAnswer(liveListing),
      "GET /filters/vocabulary": () => jsonResponse(vocabulary),
    });
    page(LIVE_ID);
    const row = (await screen.findByText("MiTek")).closest("tr") as HTMLElement;
    expect(
      screen.getByRole("columnheader", { name: "industry" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("columnheader", { name: "last touch" }),
    ).toBeInTheDocument();
    expect(await within(row).findByText("Manufacturing")).toBeInTheDocument();
    expect(
      within(row).getByText(en["lists.members.hidden"]),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /why/i })).toBeNull();
  });

  it("names a referenced record in its column rather than showing its id", async () => {
    const parent = "01a0f000-0000-7000-8000-000000000040";
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({
          ...liveList,
          definition: { field: "parent_company_id", op: "exists", value: true },
        }),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${LIVE_ID}/members`]: listingAnswer({
        ...liveListing,
        values: {
          parent_company_id: {
            value: parent,
            label: "Acme Holding",
            hidden: false,
          },
        },
      }),
      "GET /filters/vocabulary": () => jsonResponse(vocabulary),
    });
    page(LIVE_ID);
    const row = (await screen.findByText("MiTek")).closest("tr") as HTMLElement;
    expect(await within(row).findByText("Acme Holding")).toBeInTheDocument();
    expect(within(row).queryByText(parent)).toBeNull();
  });

  it("draws the first four filter fields and offers the rest under Display", async () => {
    const fields = ["industry", "city", "country", "employees", "website"];
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({
          ...liveList,
          definition: {
            and: fields.map((field) => ({ field, op: "exists", value: true })),
          },
        }),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      "GET /filters/vocabulary": () => jsonResponse(vocabulary),
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    expect(
      await screen.findByRole("columnheader", { name: "employees" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "website" })).toBeNull();
    await user.click(screen.getByRole("button", { name: en["table.display"] }));
    await user.click(screen.getByRole("checkbox", { name: "website" }));
    expect(
      screen.getByRole("columnheader", { name: "website" }),
    ).toBeInTheDocument();
  });

  it("names who chose each Shortlist member, when and why, and offers a steward to a list nobody looks after", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(shortlist),
      [`GET /lists/${SHORTLIST_ID}/history`]: () =>
        jsonResponse({ data: history, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${SHORTLIST_ID}/members`]: listingAnswer(chosenListing),
    });
    page(SHORTLIST_ID);
    expect(
      await screen.findByText(en["lists.ownerless.title"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["lists.ownerless.takeOver"] }),
    ).toBeInTheDocument();
    const row = (await screen.findByText("MiTek")).closest("tr") as HTMLElement;
    expect(await within(row).findByText("Lena Vogt")).toBeInTheDocument();
    expect(
      within(row).getByText("Signed the quote for the launch deck"),
    ).toBeInTheDocument();
    expect(within(row).getByText(/2026/)).toBeInTheDocument();
    for (const header of [
      en["lists.members.addedBy"],
      en["lists.members.addedOn"],
      en["lists.members.note"],
    ]) {
      expect(
        screen.getByRole("columnheader", { name: header }),
      ).toBeInTheDocument();
    }
    // The notice is the one place that says so: no badge repeats it, and no
    // Steward fact names the nobody it is about.
    expect(screen.queryByText(en["lists.health.ownerless"])).toBeNull();
    expect(screen.queryByText(en["lists.fact.steward"])).toBeNull();
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
    expect(screen.queryByText(en["lists.health.retiredField"])).toBeNull();
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
    expect(
      await within(await whatChanged()).findByText(/^Last checked /),
    ).toBeInTheDocument();
    expect(screen.getAllByText(/^Last checked /)).toHaveLength(1);
    expect(
      screen.getByText("Since your last visit: 3 joined, 1 left"),
    ).toBeInTheDocument();
  });

  it("reads a Live List's filter as one sentence under its purpose", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
    });
    page(LIVE_ID);
    const line = await screen.findByText(
      "Filter: Industry is Manufacturing and last touch is more than 45 days ago",
    );
    expect(line.previousElementSibling).toHaveTextContent(
      liveList.purpose ?? "",
    );
  });

  it("says nothing of a Live List's filter until the vocabulary can name its fields", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      "GET /filters/vocabulary": () => new Promise<Response>(() => {}),
    });
    page(LIVE_ID);
    expect(await screen.findByText(liveList.purpose ?? "")).toBeInTheDocument();
    expect(screen.queryByText(/^Filter: /)).toBeNull();
  });

  it("gives a Shortlist no Filter line and reads no vocabulary for one", async () => {
    let vocabularyReads = 0;
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(shortlist),
      [`POST /lists/${SHORTLIST_ID}/visit`]: visitAnswer(SHORTLIST_ID),
      [`GET /lists/${SHORTLIST_ID}/history`]: () =>
        jsonResponse({ data: history, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${SHORTLIST_ID}/members`]: listingAnswer(chosenListing),
      "GET /filters/vocabulary": () => {
        vocabularyReads++;
        return jsonResponse(vocabulary);
      },
    });
    page(SHORTLIST_ID);
    expect(await screen.findByText("MiTek")).toBeInTheDocument();
    expect(screen.getByText(shortlist.purpose ?? "")).toBeInTheDocument();
    expect(screen.queryByText(/^Filter: /)).toBeNull();
    expect(vocabularyReads).toBe(0);
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
      await within(await whatChanged()).findByText(
        en["lists.history.notChecked"],
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/^Since your last visit/),
    ).not.toBeInTheDocument();
  });

  it("credits a Live List's observed changes to the check that saw them, and says what a check cannot see when asked", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: liveHistory, page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    const region = await whatChanged();
    expect(
      await within(region).findByText("Joined · after the filter changed"),
    ).toBeInTheDocument();
    expect(
      within(region).getByText(en["lists.history.left"]),
    ).toBeInTheDocument();
    expect(screen.getAllByText(en["lists.history.checker"])).toHaveLength(2);
    expect(screen.queryByText(en["lists.history.liveNote"])).toBeNull();
    await user.click(
      within(region).getByRole("button", {
        name: en["lists.history.howChecks"],
      }),
    );
    expect(
      await screen.findByText(en["lists.history.liveNote"]),
    ).toBeInTheDocument();
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
    const region = await whatChanged();
    expect(
      await within(region).findByText("Added a record · by hand"),
    ).toBeInTheDocument();
    // The note is a line of its own under the change, joined by no dash.
    expect(
      within(region).getByText("Signed the quote for the launch deck"),
    ).toBeInTheDocument();
    expect(within(region).queryByText(/—/)).toBeNull();
    expect(
      within(region).queryByRole("button", {
        name: en["lists.history.howChecks"],
      }),
    ).toBeNull();
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

  it("names the automations an archive will pause before archiving, then hands focus to the list's name", async () => {
    const archived: string[] = [];
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({
          ...liveList,
          archived_at: archived.length > 0 ? "2026-09-30T08:00:00Z" : null,
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
      await screen.findByRole("button", { name: moreFor(liveList.name) }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.archive"] }));
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
    expect(screen.queryByText(en["lists.fact.exported"])).toBeNull();
    // The confirm closes with the menu that opened it gone, so focus has no
    // opener to return to and falls to the head that replaced it.
    expect(
      await screen.findByText(en["lists.archived.title"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1, name: liveList.name }),
    ).toHaveFocus();
  });

  it("keeps Export CSV and Edit filter in view and folds Edit list and Archive list into the list's menu", async () => {
    stubLivePage(liveList);
    const user = userEvent.setup();
    page(LIVE_ID);
    expect(
      await screen.findByRole("button", { name: en["filters.exportCsv"] }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["lists.editFilter"] }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["lists.settings"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["lists.archive"] }),
    ).toBeNull();
    const more = screen.getByRole("button", { name: moreFor(liveList.name) });
    await user.click(more);
    expect(
      within(menuOf(more))
        .getAllByRole("button")
        .map((verb) => verb.textContent),
    ).toEqual([en["lists.settings"], en["lists.archive"]]);
  });

  it("offers a reader who may not change the list its export and nothing else", async () => {
    stubLivePage({ ...liveList, can_edit: false });
    page(LIVE_ID);
    expect(
      await screen.findByRole("button", { name: en["filters.exportCsv"] }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: moreFor(liveList.name) }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["lists.editFilter"] }),
    ).toBeNull();
  });

  it("lays out how many records a reader can see, who can find the list and who looks after it", async () => {
    stubLivePage(liveList);
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.col.recordType"]),
    ).toBeInTheDocument();
    const strip = facts();
    expect(within(strip).getByText("42 companies")).toBeInTheDocument();
    expect(
      await within(strip).findByText(en["lists.audience.yourTeams"]),
    ).toBeInTheDocument();
    expect(within(strip).getByText("Lena Vogt")).toBeInTheDocument();
    expect(within(strip).queryByText(/—/)).toBeNull();
  });

  it("states a broken filter in its notice alone, and names the record type when the server could not count", async () => {
    stubLivePage({ ...liveList, health: "invalid", visible_count: null });
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.invalid.title"]),
    ).toBeInTheDocument();
    // Members are read by now, so a Select all would have been drawn.
    expect(
      await screen.findByRole("button", { name: en["filters.exportCsv"] }),
    ).toBeInTheDocument();
    expect(
      within(facts()).getByText(en["lists.type.company"]),
    ).toBeInTheDocument();
    expect(within(facts()).queryByText(/—/)).toBeNull();
    expect(screen.queryByText(en["lists.health.invalid"])).toBeNull();
    expect(screen.queryByRole("button", { name: /^Select all/ })).toBeNull();
  });

  it("dates a list's exports by the latest of them, whatever order they arrive in", async () => {
    stubLivePage({ ...liveList, dependencies: exportDependencies });
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.fact.exported"]),
    ).toBeInTheDocument();
    const when = formatDateTime("2026-09-20T14:30:00Z", "en", viewerZone());
    expect(
      within(facts()).getByText(
        en["lists.head.exported_other"]
          .replace("{count}", "2")
          .replace("{when}", when),
      ),
    ).toBeInTheDocument();
  });

  // Each verb takes its own button away by changing the notice, so the name
  // heading the fresh page is where a keyboard reader carries on from.
  it("hands focus to the list's name when archiving or restoring takes away the button pressed", async () => {
    let archivedAt: string | null = null;
    stubLivePage(liveList, {
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({ ...liveList, archived_at: archivedAt }),
      [`DELETE /lists/${LIVE_ID}`]: () => {
        archivedAt = "2026-09-30T08:00:00Z";
        return jsonResponse({ ...liveList, archived_at: archivedAt });
      },
      [`POST /lists/${LIVE_ID}/restore`]: () => {
        archivedAt = null;
        return jsonResponse(liveList);
      },
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    await user.click(
      await screen.findByRole("button", { name: moreFor(liveList.name) }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.archive"] }));
    expect(
      await screen.findByText(en["lists.archived.title"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1, name: liveList.name }),
    ).toHaveFocus();
    await user.click(screen.getByRole("button", { name: en["lists.restore"] }));
    await waitFor(() =>
      expect(screen.queryByText(en["lists.archived.title"])).toBeNull(),
    );
    expect(
      screen.getByRole("heading", { level: 1, name: liveList.name }),
    ).toHaveFocus();
  });

  it("hands focus to the list's name when taking over a list nobody looks after takes away the button pressed", async () => {
    let taken = false;
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () =>
        jsonResponse(
          taken
            ? {
                ...shortlist,
                steward_id: liveList.steward_id,
                steward_name: liveList.steward_name,
                health: "ok",
              }
            : shortlist,
        ),
      [`GET /lists/${SHORTLIST_ID}/history`]: () => jsonResponse(emptyPage),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${SHORTLIST_ID}/members`]: listingAnswer(chosenListing),
      [`PATCH /lists/${SHORTLIST_ID}`]: () => {
        taken = true;
        return jsonResponse({ ...shortlist, steward_id: liveList.steward_id });
      },
    });
    const user = userEvent.setup();
    page(SHORTLIST_ID);
    await user.click(
      await screen.findByRole("button", {
        name: en["lists.ownerless.takeOver"],
      }),
    );
    await waitFor(() =>
      expect(screen.queryByText(en["lists.ownerless.title"])).toBeNull(),
    );
    expect(
      screen.getByRole("heading", { level: 1, name: shortlist.name }),
    ).toHaveFocus();
  });

  it("still names the health and the missing steward of an archived list, whose notice says only that it is archived", async () => {
    stubLivePage({
      ...liveList,
      archived_at: "2026-09-30T08:00:00Z",
      health: "ownerless",
      steward_id: null,
      steward_name: null,
    });
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.archived.title"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(en["lists.ownerless.title"])).toBeNull();
    expect(screen.getAllByText(en["lists.health.ownerless"])).toHaveLength(1);
    expect(within(facts()).getByText(en["lists.fact.steward"])).toBeVisible();
    expect(within(facts()).getByText(en["lists.noSteward"])).toBeVisible();
  });

  it("says an archived list's broken filter once, beside its name", async () => {
    stubLivePage({
      ...liveList,
      archived_at: "2026-09-30T08:00:00Z",
      health: "invalid",
      visible_count: null,
    });
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.archived.title"]),
    ).toBeInTheDocument();
    expect(screen.getAllByText(en["lists.health.invalid"])).toHaveLength(1);
    expect(screen.queryByText(en["lists.invalid.title"])).toBeNull();
  });

  it("opens the archive dialog to say why when an archive with no rules to name fails", async () => {
    stubLivePage(liveList, {
      [`DELETE /lists/${LIVE_ID}`]: () =>
        jsonResponse({ title: "Conflict", status: 409 }, 409),
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    await user.click(
      await screen.findByRole("button", { name: moreFor(liveList.name) }),
    );
    await user.click(screen.getByRole("button", { name: en["lists.archive"] }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("alert")).toBeInTheDocument();
    expect(
      within(dialog).getByRole("button", { name: en["lists.archive"] }),
    ).toBeEnabled();
  });

  it("says in the notice why Restore failed", async () => {
    const archived = { ...liveList, archived_at: "2026-09-30T08:00:00Z" };
    stubLivePage(archived, {
      [`POST /lists/${LIVE_ID}/restore`]: () =>
        jsonResponse({ title: "Forbidden", status: 403 }, 403),
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    await user.click(
      await screen.findByRole("button", { name: en["lists.restore"] }),
    );
    const notice = screen
      .getByText(en["lists.archived.title"])
      .closest(".callout");
    if (!(notice instanceof HTMLElement)) {
      throw new Error("the archived title sits in no notice");
    }
    expect(await within(notice).findByRole("alert")).toBeInTheDocument();
  });

  it("says under the list's name why an export failed", async () => {
    stubLivePage(liveList, {
      "POST /exports": () =>
        jsonResponse({ title: "Forbidden", status: 403 }, 403),
    });
    const user = userEvent.setup();
    page(LIVE_ID);
    await user.click(
      await screen.findByRole("button", { name: en["filters.exportCsv"] }),
    );
    const head = screen
      .getByRole("heading", { level: 1, name: liveList.name })
      .closest("header");
    if (!(head instanceof HTMLElement)) {
      throw new Error("the list's name sits in no header");
    }
    expect(await within(head).findByRole("alert")).toBeInTheDocument();
  });

  it("counts a finished export in the Exported fact without a reload", async () => {
    let exported: NonNullable<List["dependencies"]> = [];
    stubLivePage(liveList, {
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({ ...liveList, dependencies: exported }),
      "POST /exports": () => {
        exported = [exportDependencies[1]];
        return new Response("id,name\n", {
          status: 200,
          headers: { "Content-Type": "text/csv" },
        });
      },
    });
    Object.defineProperties(URL, {
      createObjectURL: { configurable: true, value: vi.fn(() => "blob:test") },
      revokeObjectURL: { configurable: true, value: vi.fn() },
    });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      () => undefined,
    );
    const user = userEvent.setup();
    page(LIVE_ID);
    await user.click(
      await screen.findByRole("button", { name: en["filters.exportCsv"] }),
    );
    expect(
      await screen.findByText(en["lists.fact.exported"]),
    ).toBeInTheDocument();
  });

  it("reads no members for a list of projects and offers no export of them", async () => {
    const memberReads: string[] = [];
    const recordRead = (path: string) => () => {
      memberReads.push(path);
      return jsonResponse(emptyPage);
    };
    stubLivePage(
      { ...liveList, entity_type: "project" },
      {
        "GET /contacts": recordRead("/contacts"),
        "GET /companies": recordRead("/companies"),
        "GET /deals": recordRead("/deals"),
        "GET /leads": recordRead("/leads"),
      },
    );
    page(LIVE_ID);
    expect(
      await screen.findByText(en["lists.members.projects"]),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["filters.exportCsv"] }),
    ).toBeNull();
    expect(memberReads).toEqual([]);
  });
});
