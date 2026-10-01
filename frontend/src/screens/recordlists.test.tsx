// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
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
import { recordWriteKeys } from "./recordwritekeys";
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

  it("says when more lists hold the record than the answer carries", async () => {
    stub({
      [`GET /records/company/${MEMBER_ID}/lists`]: () =>
        jsonResponse({ data: [editableShortlist], truncated: true }),
    });
    expect(
      await screen.findByText(en["lists.record.truncated"]),
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

  it("names a referenced record in a clause rather than showing its id", async () => {
    const parent = "01a0f000-0000-7000-8000-000000000040";
    stub({
      [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse({
          ...notOnLiveWhy,
          clauses: {
            field: "parent_company_id",
            op: "exists",
            operand: true,
            result: true,
            value: parent,
            value_label: "Acme Holding",
          },
        }),
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("combobox", { name: en["lists.record.check"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: liveList.name }),
    );
    expect(await screen.findByText("Now: Acme Holding")).toBeInTheDocument();
    expect(screen.queryByText(`Now: ${parent}`)).toBeNull();
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

  it("reads the record's lists and an open verdict again when the record is written", async () => {
    let listReads = 0;
    let whyReads = 0;
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /records/company/${MEMBER_ID}/lists`]: () => {
        listReads += 1;
        return jsonResponse({ data: [] });
      },
      "GET /lists": () =>
        jsonResponse({ data: [liveList], page: { has_more: false } }),
      [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () => {
        whyReads += 1;
        return jsonResponse(notOnLiveWhy);
      },
      "GET /filters/vocabulary": () =>
        jsonResponse({ resource: "company", fields: [] }),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <RecordListsPanel entityType="company" entityId={MEMBER_ID} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("combobox", { name: en["lists.record.check"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: liveList.name }),
    );
    await screen.findByText("Now: Logistics");
    expect([listReads, whyReads]).toEqual([1, 1]);
    await Promise.all(
      recordWriteKeys("company", MEMBER_ID).map((queryKey) =>
        client.invalidateQueries({ queryKey }),
      ),
    );
    await vi.waitFor(() => expect([listReads, whyReads]).toEqual([2, 2]));
  });
});
