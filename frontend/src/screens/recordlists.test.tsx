// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import {
  LIVE_ID,
  listsMe,
  liveList,
  MEMBER_ID,
  notOnLiveWhy,
  SHORTLIST_ID,
  shortlist,
} from "./lists.fixtures";
import { RecordListsPanel } from "./recordlists";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const editableShortlist = { ...shortlist, health: "ok" as const };

function stub(extra: Parameters<typeof installFetchStub>[0] = {}) {
  installFetchStub({
    "GET /me": listsMe(true),
    [`GET /records/company/${MEMBER_ID}/lists`]: () =>
      jsonResponse({ data: [editableShortlist] }),
    "GET /lists": () =>
      jsonResponse({ data: [liveList], page: { has_more: false } }),
    "GET /filters/vocabulary": () =>
      jsonResponse({ resource: "company", fields: [] }),
    ...extra,
  });
  render(
    <StoryProviders>
      <RecordListsPanel entityType="company" entityId={MEMBER_ID} />
    </StoryProviders>,
  );
}

describe("a record page's lists", () => {
  it("names the lists the record is on, each by its kind", async () => {
    stub({
      [`GET /records/company/${MEMBER_ID}/lists`]: () =>
        jsonResponse({ data: [editableShortlist, liveList] }),
    });
    const shortlistRow = (
      await screen.findByRole("button", { name: shortlist.name })
    ).closest("li") as HTMLElement;
    expect(
      within(shortlistRow).getByText(en["lists.kind.shortlist"]),
    ).toBeInTheDocument();
    const liveRow = screen
      .getByRole("button", { name: liveList.name })
      .closest("li") as HTMLElement;
    expect(
      within(liveRow).getByText(en["lists.kind.live"]),
    ).toBeInTheDocument();
  });

  it("says it is on no list the reader can find", async () => {
    stub({
      [`GET /records/company/${MEMBER_ID}/lists`]: () =>
        jsonResponse({ data: [] }),
    });
    expect(
      await screen.findByText(en["lists.record.empty"]),
    ).toBeInTheDocument();
  });

  it("explains which clause keeps the record off a Live List, with its value", async () => {
    stub({
      [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse(notOnLiveWhy),
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("combobox", { name: en["lists.record.check"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: liveList.name }),
    );
    expect(
      await screen.findByText(en["lists.why.liveNotMember"]),
    ).toBeInTheDocument();
    const failing = screen.getByText("Now: Logistics").closest("p");
    expect(
      within(failing as HTMLElement).getByLabelText(en["lists.why.unmet"]),
    ).toBeInTheDocument();
    expect(screen.getByText(en["lists.why.hidden"])).toBeInTheDocument();
  });

  it("takes the record off a Shortlist with a note", async () => {
    const removed: unknown[] = [];
    stub({
      [`POST /lists/${SHORTLIST_ID}/members/remove`]: (body) => {
        removed.push(body);
        return new Response(null, { status: 204 });
      },
    });
    const user = userEvent.setup();
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
