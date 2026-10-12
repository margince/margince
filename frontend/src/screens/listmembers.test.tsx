// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { ListScreen } from "./listpage";
import {
  LIVE_ID,
  listsMe,
  liveList,
  MEMBER_ID,
  members,
  SHORTLIST_ID,
  shortlist,
} from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// A list's members are acted on as explicit records: the reader ticks them, or
// selects every member the list shows them, and the bulk dialog previews the
// change for exactly those ids.

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(URL, "createObjectURL");
  Reflect.deleteProperty(URL, "revokeObjectURL");
});

const emptyPreview = {
  record_type: "company",
  verb: "remove_from_list",
  count: 0,
  affected: [],
  excluded: [],
  sample: [],
  requires_confirmation: false,
};

type Routes = Parameters<typeof installFetchStub>[0];

function openList(listID: string, routes: Routes) {
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
    [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(shortlist),
    [`GET /lists/${LIVE_ID}/history`]: () =>
      jsonResponse({ data: [], page: { has_more: false } }),
    [`GET /lists/${SHORTLIST_ID}/history`]: () =>
      jsonResponse({ data: [], page: { has_more: false } }),
    "GET /companies": () =>
      jsonResponse({ data: members, page: { has_more: false } }),
    ...routes,
  });
  return render(
    <StoryProviders>
      <ListScreen listID={listID} />
    </StoryProviders>,
  );
}

const selectMiTek = en["bulk.selectRow"].replace("{name}", "MiTek");

describe("acting on a list's members", () => {
  it("offers the bulk verbs over a ticked member, and no Shortlist removal on a Live List", async () => {
    const user = userEvent.setup();
    openList(LIVE_ID, {});
    await user.click(
      await screen.findByRole("checkbox", { name: selectMiTek }),
    );
    expect(
      screen.getByRole("button", { name: en["bulk.assign"] }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["bulk.createTask"] }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: en["bulk.archive"] }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: en["bulk.removeFromThisShortlist"],
      }),
    ).not.toBeInTheDocument();
  });

  it("takes a ticked member off this Shortlist through remove_from_list", async () => {
    const previews: unknown[] = [];
    const user = userEvent.setup();
    openList(SHORTLIST_ID, {
      "POST /bulk/preview": (body) => {
        previews.push(body);
        return jsonResponse(emptyPreview);
      },
    });
    await user.click(
      await screen.findByRole("checkbox", { name: selectMiTek }),
    );
    await user.click(
      screen.getByRole("button", { name: en["bulk.removeFromThisShortlist"] }),
    );
    await waitFor(() => expect(previews).toHaveLength(1));
    expect(previews[0]).toMatchObject({
      record_type: "company",
      verb: "remove_from_list",
      list_id: SHORTLIST_ID,
      items: [{ id: MEMBER_ID, version: 4 }],
    });
  });

  it("files a task under the ticked members once the reader says what it is", async () => {
    const previews: unknown[] = [];
    const user = userEvent.setup();
    openList(LIVE_ID, {
      "POST /bulk/preview": (body) => {
        previews.push(body);
        return jsonResponse({ ...emptyPreview, verb: "create_task" });
      },
    });
    await user.click(
      await screen.findByRole("checkbox", { name: selectMiTek }),
    );
    await user.click(
      screen.getByRole("button", { name: en["bulk.createTask"] }),
    );
    await user.type(
      await screen.findByRole("textbox", { name: en["bulk.taskSubject"] }),
      "Send the renewal quote",
    );
    await user.click(screen.getByRole("button", { name: en["bulk.taskNext"] }));
    await waitFor(() => expect(previews).toHaveLength(1));
    expect(previews[0]).toMatchObject({
      verb: "create_task",
      task: { subject: "Send the renewal quote" },
      items: [{ id: MEMBER_ID }],
    });
  });

  it("selects every member up to the bulk cap and says when more remain", async () => {
    let walked = 0;
    const page = (from: number) =>
      Array.from({ length: 200 }, (_, i) => ({
        id: `01a0f000-0000-7000-8000-${String(from + i).padStart(12, "0")}`,
        display_name: `Company ${from + i}`,
        version: 1,
      }));
    const user = userEvent.setup();
    openList(LIVE_ID, {
      "GET /companies": () => {
        walked += 1;
        return jsonResponse({
          data: walked === 1 ? members : page(walked * 1000),
          page: { has_more: true, next_cursor: `c${walked}` },
        });
      },
    });
    await screen.findByText("MiTek");
    await user.click(
      screen.getByRole("button", {
        name: en["lists.members.selectAll_other"].replace("{count}", "42"),
      }),
    );
    expect(
      await screen.findByText(en["lists.members.selectAllCappedTitle"]),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        en["lists.members.selectAllCapped_other"].replace("{count}", "500"),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(en["bulk.selected_other"].replace("{count}", "500")),
    ).toBeInTheDocument();
  });

  it("adds no member past the cap but still lets one be unticked", async () => {
    let walked = 0;
    const others = (from: number, n: number) =>
      Array.from({ length: n }, (_, i) => ({
        id: `01a0f000-0000-7000-8000-${String(from + i).padStart(12, "0")}`,
        display_name: `Company ${from + i}`,
        version: 1,
      }));
    const user = userEvent.setup();
    openList(LIVE_ID, {
      "GET /companies": () => {
        walked += 1;
        const data =
          walked === 1
            ? members
            : walked === 2
              ? [members[0], ...others(1000, 199)]
              : others(walked * 1000, 200);
        return jsonResponse({
          data,
          page: { has_more: walked < 4, next_cursor: `c${walked}` },
        });
      },
    });
    await screen.findByText("MiTek");
    await user.click(
      screen.getByRole("button", {
        name: en["lists.members.selectAll_other"].replace("{count}", "42"),
      }),
    );
    const selected = (n: number) =>
      en["bulk.selected_other"].replace("{count}", String(n));
    expect(await screen.findByText(selected(500))).toBeInTheDocument();
    const nordfracht = en["bulk.selectRow"].replace("{name}", "Nordfracht");
    await user.click(screen.getByRole("checkbox", { name: nordfracht }));
    expect(screen.getByText(selected(500))).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: nordfracht }),
    ).not.toBeChecked();
    expect(
      screen.getByText(en["lists.members.selectionFullTitle"]),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: selectMiTek }));
    expect(await screen.findByText(selected(499))).toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: nordfracht }));
    expect(await screen.findByText(selected(500))).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: nordfracht })).toBeChecked();
  });

  it("exports the list's members through the list's own export", async () => {
    const exports: unknown[] = [];
    const user = userEvent.setup();
    openList(SHORTLIST_ID, {
      "POST /exports": (body) => {
        exports.push(body);
        return new Response("id,display_name\n", {
          status: 200,
          headers: { "Content-Type": "text/csv" },
        });
      },
    });
    await screen.findByText("MiTek");
    const createObjectURL = vi.fn(() => "blob:test");
    Object.defineProperties(URL, {
      createObjectURL: { configurable: true, value: createObjectURL },
      revokeObjectURL: { configurable: true, value: vi.fn() },
    });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      () => undefined,
    );
    await user.click(
      screen.getByRole("button", { name: en["filters.exportCsv"] }),
    );
    await waitFor(() => expect(exports).toHaveLength(1));
    expect(exports[0]).toEqual({ list_id: SHORTLIST_ID, format: "csv" });
    await waitFor(() => expect(createObjectURL).toHaveBeenCalledOnce());
  });
});
