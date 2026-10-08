// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import {
  LIVE_ID,
  listsMe,
  liveList,
  liveVocabulary,
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
      "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
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
    // The clause reads as the list's own Filter line reads it, operand marked.
    const ago = await screen.findByText("45 days ago", { selector: "strong" });
    expect(ago.parentElement).toHaveTextContent(
      /^last touch is more than 45 days ago$/,
    );
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
      "GET /filters/vocabulary": () =>
        jsonResponse({
          resource: "company",
          fields: [
            {
              name: "parent_company_id",
              type: "id",
              operators: ["exists"],
              custom: false,
              references: "company",
            },
          ],
        }),
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("combobox", { name: en["lists.record.check"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: liveList.name }),
    );
    const value = await screen.findByText("Now: Acme Holding");
    expect(screen.queryByText(`Now: ${parent}`)).toBeNull();
    // An exists clause says it all in its operator, so no operand is marked.
    const clause = value.closest("p") as HTMLElement;
    expect(await within(clause).findByText(/has a value$/)).toBeInTheDocument();
    expect(clause.querySelector("strong")).toBeNull();
  });

  it.each([
    ["in the currency its field names", { currency: "EUR" }, "Now: €1,250.00"],
    ["as nothing yet when its field names no currency", {}, "Now: …"],
  ])(
    "reads a money value %s, never its minor units as the amount",
    async (_, named, shown) => {
      stub({
        [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
          jsonResponse({
            ...notOnLiveWhy,
            clauses: {
              field: "revenue",
              op: "gt",
              operand: 100000,
              result: true,
              value: "125000",
            },
          }),
        "GET /filters/vocabulary": () =>
          jsonResponse({
            resource: "company",
            fields: [
              {
                name: "revenue",
                type: "currency",
                operators: ["gt"],
                custom: false,
                ...named,
              },
            ],
          }),
      });
      const user = userEvent.setup();
      await user.click(
        await screen.findByRole("combobox", { name: en["lists.record.check"] }),
      );
      await user.click(
        await screen.findByRole("option", { name: liveList.name }),
      );
      expect(await screen.findByText(shown)).toBeInTheDocument();
      expect(screen.queryByText("Now: 125000")).toBeNull();
    },
  );

  it("names a retired tag in a clause and says no record carries one", async () => {
    const live = "01a0f000-0000-7000-8000-000000000050";
    const archived = "01a0f000-0000-7000-8000-000000000051";
    stub({
      "GET /me": () =>
        jsonResponse({
          ...meFixture({
            allow: { list: ["read"], company: ["read"], tag: ["read"] },
            settingsAvailability: { lists: true },
          }),
          teams: [],
        }),
      "GET /tags": () =>
        jsonResponse({
          data: [
            { id: live, name: "Key account" },
            {
              id: archived,
              name: "Trade fair 2025",
              archived_at: "2026-01-12T09:00:00Z",
            },
          ],
          page: { has_more: false },
        }),
      [`GET /lists/${LIVE_ID}/members/${MEMBER_ID}/why`]: () =>
        jsonResponse({
          ...notOnLiveWhy,
          clauses: {
            join: "or",
            result: false,
            children: [
              {
                field: "tag",
                op: "eq",
                operand: live,
                result: false,
                hidden: true,
              },
              {
                field: "tag",
                op: "eq",
                operand: archived,
                result: false,
                hidden: true,
              },
            ],
          },
        }),
      "GET /filters/vocabulary": () =>
        jsonResponse({
          resource: "company",
          fields: [
            {
              name: "tag",
              type: "id",
              operators: ["eq", "neq", "in"],
              custom: false,
              references: "tag",
            },
          ],
        }),
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("combobox", { name: en["lists.record.check"] }),
    );
    await user.click(
      await screen.findByRole("option", { name: liveList.name }),
    );
    const retired = await screen.findByText("1 archived tag", {
      selector: "strong",
    });
    expect(retired.closest("p")).toHaveTextContent(
      en["filters.sentence.retiredTagNote"],
    );
    // The live tag's clause beside it is an ordinary one, with no such note.
    const counted = screen.getByText("1 tag", { selector: "strong" });
    expect(counted.closest("p")).not.toHaveTextContent(
      en["filters.sentence.retiredTagNote"],
    );
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

// Taking a record off a Shortlist runs at once; the toast's Undo hands the
// removal's audit id to the restore route.
describe("taking a record off a Shortlist", () => {
  const AUDIT = "0199a000-0000-7000-8000-0000000000b2";
  const REMOVE = `POST /lists/${SHORTLIST_ID}/members/remove`;
  const RESTORE = `POST /lists/${SHORTLIST_ID}/members/restore`;
  const takenOff = en["lists.record.takenOff"].replace(
    "{name}",
    shortlist.name,
  );

  function serve(
    answers: Partial<Record<"remove" | "restore", () => Response>> = {},
  ) {
    let onList = true;
    const sent: { remove: unknown[]; restore: unknown[] } = {
      remove: [],
      restore: [],
    };
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /records/company/${MEMBER_ID}/lists`]: () =>
        jsonResponse({ data: onList ? [editableShortlist] : [] }),
      "GET /lists": () =>
        jsonResponse({ data: [liveList], page: { has_more: false } }),
      "GET /filters/vocabulary": () =>
        jsonResponse({ resource: "company", fields: [] }),
      [REMOVE]: (body) => {
        sent.remove.push(body);
        const answer = answers.remove?.();
        if (answer) {
          return answer;
        }
        onList = false;
        return jsonResponse({ audit_id: AUDIT });
      },
      [RESTORE]: (body) => {
        sent.restore.push(body);
        const answer = answers.restore?.();
        if (answer) {
          return answer;
        }
        onList = true;
        return jsonResponse({});
      },
    });
    render(
      <StoryProviders>
        <ToastProvider>
          <RecordListsPanel entityType="company" entityId={MEMBER_ID} />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>,
    );
    return sent;
  }

  const refused = () =>
    jsonResponse({ detail: "The record was added to this list again." }, 409);

  it("takes it off at once, with no dialog and no note, and offers Undo", async () => {
    const sent = serve();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: en["lists.remove"] }),
    );

    const said = await screen.findByRole("status");
    await waitFor(() => expect(said).toHaveTextContent(takenOff));
    expect(sent.remove).toEqual([
      { entity_type: "company", entity_id: MEMBER_ID },
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      within(said).getByRole("button", { name: en["common.undo"] }),
    ).toBeInTheDocument();
  });

  it("hands focus to the lists block once the row is gone", async () => {
    serve();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: en["lists.remove"] }),
    );

    expect(
      await screen.findByText(en["lists.record.empty"]),
    ).toBeInTheDocument();
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement).toContainElement(
      screen.getByText(en["lists.record.empty"]),
    );
  });

  it("puts it back through Undo with the removal's audit id", async () => {
    const sent = serve();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["lists.remove"] }),
    );
    const said = await screen.findByRole("status");

    await user.click(
      await within(said).findByRole("button", { name: en["common.undo"] }),
    );

    await waitFor(() => expect(sent.restore).toEqual([{ audit_id: AUDIT }]));
    expect(
      await screen.findByRole("button", { name: shortlist.name }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        en["lists.record.putBack"].replace("{name}", shortlist.name),
      ),
    );
  });

  it("keeps a refused removal on screen as a danger toast", async () => {
    serve({ remove: refused });
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: en["lists.remove"] }),
    );

    const said = await screen.findByRole("status");
    await waitFor(() =>
      expect(said).toHaveTextContent(
        "The record was added to this list again.",
      ),
    );
    expect(said.querySelector(".toast-dot-danger")).not.toBeNull();
    expect(
      within(said).getByRole("button", { name: en["common.close"] }),
    ).toBeInTheDocument();
  });

  it("keeps a refused Undo on screen as a danger toast", async () => {
    serve({ restore: refused });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["lists.remove"] }),
    );
    const said = await screen.findByRole("status");

    await user.click(
      await within(said).findByRole("button", { name: en["common.undo"] }),
    );

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "The record was added to this list again.",
      ),
    );
    expect(
      screen.getByRole("status").querySelector(".toast-dot-danger"),
    ).not.toBeNull();
  });
});
