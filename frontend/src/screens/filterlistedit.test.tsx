// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { FiltersScreen } from "./filters";
import { ListScreen } from "./listpage";
import { LIVE_ID, listsMe, liveList } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const COMPANY_VOCAB = {
  resource: "company",
  fields: [
    { name: "industry", type: "text", operators: ["eq", "neq"], custom: false },
    {
      name: "cf_last_touch",
      type: "date",
      operators: ["lt", "gt"],
      custom: true,
    },
  ],
};

const PREVIEW = {
  resource: "company",
  match_count: 42,
  columns: ["id"],
  rows: [],
  truncated: false,
};

const saveTo = en["lists.saveFilterTo"].replace("{name}", liveList.name);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function builder(
  list = liveList,
  patch = (_body: unknown) => jsonResponse(list),
) {
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /lists/${LIVE_ID}`]: () => jsonResponse(list),
    "GET /filters/vocabulary": () => jsonResponse(COMPANY_VOCAB),
    "POST /filters/preview": () => jsonResponse(PREVIEW),
    [`PATCH /lists/${LIVE_ID}`]: patch,
  });
  return render(
    <StoryProviders>
      <FiltersScreen id="list" view={LIVE_ID} />
    </StoryProviders>,
  );
}

describe("editing a Live List's filter", () => {
  it("opens the builder on the list's filter and saves it with the version it opened", async () => {
    const written: unknown[] = [];
    builder(liveList, (body) => {
      written.push(body);
      return jsonResponse({ ...liveList, version: 4 });
    });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: saveTo }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", {
        name: en["lists.saveFilterConfirm"],
      }),
    );
    await vi.waitFor(() => expect(written).toHaveLength(1));
    expect(written[0]).toEqual({
      version: liveList.version,
      definition: liveList.definition,
    });
    await vi.waitFor(() =>
      expect(window.location.hash).toBe(`#/lists/${LIVE_ID}`),
    );
  });

  it("names the automations watching the list before the save", async () => {
    builder({
      ...liveList,
      dependencies: [
        {
          kind: "automation",
          occurred_at: "2026-09-02T00:00:00Z",
          blocking: false,
          role: "watches",
          automation_id: "01a0f000-0000-7000-8000-000000000041",
          automation_name: "Tell me about new buyers",
        },
      ],
    });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: saveTo }));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(en["lists.rules.settingsLeadLive"]),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        en["lists.rules.watches"].replace("{name}", "Tell me about new buyers"),
      ),
    ).toBeInTheDocument();
  });

  it("says plainly when somebody changed the list since it was opened", async () => {
    builder(liveList, () =>
      jsonResponse(
        { code: "version_skew", title: "Conflict", status: 409 },
        409,
      ),
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: saveTo }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", {
        name: en["lists.saveFilterConfirm"],
      }),
    );
    expect(
      await within(dialog).findByText(en["lists.saveFilterConflict"]),
    ).toBeInTheDocument();
  });

  it("offers no save to a list the reader may not change", async () => {
    builder({ ...liveList, can_edit: false });
    await screen.findByText(en["filters.builderTitle"]);
    await vi.waitFor(() =>
      expect(
        screen.getAllByRole("button", { name: en["filters.saveList"] }),
      ).not.toHaveLength(0),
    );
    expect(screen.queryByRole("button", { name: saveTo })).toBeNull();
  });
});

describe("the Edit filter action on a list page", () => {
  function page(list = liveList) {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(list),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: [], page: { has_more: false } }),
      "GET /companies": () =>
        jsonResponse({ data: [], page: { has_more: false } }),
    });
    return render(
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>,
    );
  }

  it("opens the builder on this list for its steward", async () => {
    page();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["lists.editFilter"] }),
    );
    expect(window.location.hash).toBe(`#/filters/list/${LIVE_ID}`);
  });

  it("is offered from the retired-field notice too", async () => {
    page({ ...liveList, health: "retired_field", retired_fields: ["cf_x"] });
    await screen.findByText(en["lists.retiredField.title"]);
    expect(
      screen.getAllByRole("button", { name: en["lists.editFilter"] }),
    ).toHaveLength(2);
  });

  it("is not offered to a reader who may not change the list", async () => {
    page({ ...liveList, can_edit: false });
    await screen.findByText(liveList.name);
    expect(
      screen.queryByRole("button", { name: en["lists.editFilter"] }),
    ).toBeNull();
  });
});
