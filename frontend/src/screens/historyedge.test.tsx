/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { entryFieldChanges } from "./history.logic";
import { RecordHistory } from "./historyentries";
import { historyRows, netChanges } from "./historyreversal";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
  globalThis.location.hash = "";
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const RESTORE = { version: 7, onRestored: () => {} };

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

const employer: NonNullable<components["schemas"]["HistoryEdge"]> = {
  kind: "employment",
  other_entity_type: "organization",
  other_entity_id: "org-9",
  other_label: "Employer GmbH",
};

// A link created between this person and a company. The images are the EDGE's
// columns, which is exactly the shape that must not reach a field diff.
const linked: AuditHistoryEntry = {
  id: "h1",
  actor_type: "human",
  actor_id: "human:u1",
  actor_name: "Ada Admin",
  action: "create",
  occurred_at: "2026-07-14T10:00:00Z",
  summary: "Ada Admin linked Employer GmbH as cto",
  before: null,
  after: { role: "cto", started_at: "2026-01-01", is_primary: true },
  edge: employer,
  undoable: { undoable: true, reason: null, detail: null },
};

function servingHistory(
  entries: readonly unknown[],
  onOtherLookup?: () => Response,
) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/organizations/")) {
      if (!onOtherLookup) {
        throw new Error(`unexpected endpoint lookup: ${url}`);
      }
      return onOtherLookup();
    }
    return jsonResponse({ data: entries, page: { next_cursor: null } });
  });
}

describe("an edge row in a record's history", () => {
  // `role`, `started_at` and the primary flag belong to the LINK. Drawn as this
  // record's fields they invent fields it does not have, and the label map has
  // no word for any of them.
  it("renders the server's sentence and no field diff", async () => {
    vi.stubGlobal("fetch", servingHistory([linked]));
    const { container } = render(
      <RecordHistory kind="person" id="p1" restore={RESTORE} />,
    );

    expect(
      await screen.findByText("Ada Admin linked Employer GmbH as cto"),
    ).toBeTruthy();
    expect(container.querySelector(".entry-fields")).toBeNull();
    expect(screen.queryByText("role")).toBeNull();
    expect(screen.queryByText("started at")).toBeNull();
    expect(screen.queryByText("cto")).toBeNull();
  });

  it("takes the reader to the record at the other end", async () => {
    vi.stubGlobal("fetch", servingHistory([linked]));
    const user = userEvent.setup();
    render(<RecordHistory kind="person" id="p1" restore={RESTORE} />);

    await user.click(
      await screen.findByRole("button", { name: "Employer GmbH" }),
    );

    expect(globalThis.location.hash).toBe("#/companies/org-9");
  });

  // A read that could not name the other end says so. Neither the word "null"
  // nor a gap where a record belongs: both read as a link to nothing.
  it("names an endpoint the read could not resolve", async () => {
    vi.stubGlobal(
      "fetch",
      servingHistory(
        [{ ...linked, edge: { ...employer, other_label: null } }],
        () => jsonResponse({ title: "boom" }, 500),
      ),
    );
    const { container } = render(
      <RecordHistory kind="person" id="p1" restore={RESTORE} />,
    );

    expect(await screen.findByText(/name didn't load/i)).toBeTruthy();
    expect(screen.queryByText("null")).toBeNull();
    expect(container.querySelector(".entry-edge")?.textContent).toBeTruthy();
  });

  // Undoing an unlink is an unarchive, which this product does not offer yet —
  // so the row is visible and the refusal names what the reader CAN do.
  it("refuses an unlink with the relink sentence", async () => {
    vi.stubGlobal(
      "fetch",
      servingHistory([
        {
          ...linked,
          id: "h2",
          action: "archive",
          summary: "Ada Admin unlinked Employer GmbH",
          undoable: {
            undoable: false,
            reason: "edge_relink_unsupported",
            detail: null,
          },
        },
      ]),
    );
    render(<RecordHistory kind="person" id="p1" restore={RESTORE} />);

    const button = await screen.findByRole("button", { name: /put back/i });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(/add it again on this record/i)).toBeTruthy();
  });

  // A link change must be tellable from a field edit before a word of either is
  // read, or a record with many contacts reads as one undifferentiated list.
  it("marks the row as a link rather than a field change", async () => {
    vi.stubGlobal("fetch", servingHistory([linked]));
    const { container } = render(
      <RecordHistory kind="person" id="p1" restore={RESTORE} />,
    );

    await screen.findByText("Ada Admin linked Employer GmbH as cto");
    expect(container.querySelector(".entry-edge")).toBeTruthy();
  });
});

// The pairing keys on `undid_audit_log_id`, which an edge reversal carries like
// any other. Held here so a pairing that grew a record-only assumption fails on
// the edge case rather than on nobody.
describe("an edge reversal pairs with what it reversed", () => {
  it("collapses the pair exactly as a record row does", () => {
    const reversal: AuditHistoryEntry = {
      ...linked,
      id: "h2",
      action: "archive",
      occurred_at: "2026-07-15T10:00:00Z",
      summary: "Bo Boss undid the link to Employer GmbH",
      actor_id: "human:u2",
      actor_name: "Bo Boss",
      undid_audit_log_id: "h1",
      before: { role: "cto" },
      after: { role: null },
    };

    const rows = historyRows([reversal, linked]);

    expect(rows).toHaveLength(1);
    const [row] = rows;
    expect(row.kind).toBe("pair");
    if (row.kind !== "pair") {
      throw new Error("expected the edge reversal to pair");
    }
    expect(row.reversal.id).toBe("h2");
    expect(row.reversed.id).toBe("h1");
    expect(row.sameActor).toBe(false);
  });
});

describe("an ordinary field row is unaffected", () => {
  it("still draws its field diff", async () => {
    vi.stubGlobal(
      "fetch",
      servingHistory([
        {
          ...linked,
          action: "update",
          summary: "Ada Admin updated the record",
          edge: null,
          before: { title: "CTO" },
          after: { title: "CEO" },
        },
      ]),
    );
    const { container } = render(
      <RecordHistory kind="person" id="p1" restore={RESTORE} />,
    );

    expect(await screen.findByText("Job title")).toBeTruthy();
    expect(container.querySelector(".entry-edge")).toBeNull();
  });
});

// Waiting on nothing: the suite must not pass because a render never happened.
describe("the harness", () => {
  it("serves the history the rows are read from", async () => {
    const fetchMock = servingHistory([linked]);
    vi.stubGlobal("fetch", fetchMock);
    render(<RecordHistory kind="person" id="p1" restore={RESTORE} />);

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
  });
});

// A collapsed pair reports its net over the SAME rule the row uses.
//
// This is the case a fix at the row's call site would have missed: the pair's
// face and the net it reports go through entryFieldChanges too, so a link's own
// columns would surface there under labels the record has no fields for — the
// row clean, the pair face wrong.
describe("a link's columns never reach a pair's face", () => {
  const linkMade: AuditHistoryEntry = {
    id: "e1",
    actor_type: "human",
    actor_id: "human:u-1",
    actor_name: "Ada Admin",
    action: "create",
    occurred_at: "2026-08-27T10:22:00Z",
    summary: "Ada Admin linked Employer GmbH as cto",
    before: null,
    after: { role: "cto", started_at: "2026-01-01", is_primary: true },
    edge: {
      kind: "employment",
      other_entity_type: "organization",
      other_entity_id: "11111111-1111-1111-1111-111111111111",
      other_label: "Employer GmbH",
    },
  };
  const linkReversed: AuditHistoryEntry = {
    id: "e2",
    actor_type: "human",
    actor_id: "human:u-2",
    actor_name: "Tin Nguyen",
    action: "restore",
    occurred_at: "2026-08-27T11:00:00Z",
    summary: "Tin Nguyen put the link back",
    undid_audit_log_id: "e1",
    before: { role: "cto" },
    after: { role: null },
    edge: {
      kind: "employment",
      other_entity_type: "organization",
      other_entity_id: "11111111-1111-1111-1111-111111111111",
      other_label: "Employer GmbH",
    },
  };

  it("reports no field changes for either half", () => {
    expect(entryFieldChanges(linkMade)).toEqual([]);
    expect(entryFieldChanges(linkReversed)).toEqual([]);
  });

  it("reports an empty net, so the pair does not claim a link's columns moved", () => {
    const [row] = historyRows([linkReversed, linkMade]);
    expect(row.kind).toBe("pair");
    if (row.kind !== "pair") return;
    expect(netChanges(row)).toEqual([]);
    expect(row.whollyUndone).toBe(true);
  });
});

// Reversing a link asks first.
//
// The confirm gate used to key on the number of fields a change moved, and an
// edge entry moves none — so a two-field edit asked and REMOVING A LINK BETWEEN
// TWO RECORDS did not. The more consequential of the two was the unconfirmed one.
describe("reversing a link asks before it writes", () => {
  it("opens a confirmation naming the other record, and does not write on the first press", async () => {
    const fetchMock = servingHistory([linked]);
    vi.stubGlobal("fetch", fetchMock);
    render(<RecordHistory kind="person" id="p-1" restore={RESTORE} />);
    const button = await screen.findByRole("button", { name: /put back/i });
    await userEvent.click(button);

    // The dialog names the record at the other end, so the reader knows which
    // connection they are about to change. Matched on the dialog's own sentence
    // because the ROW names the company too — asserting on the name alone would
    // pass on a page that never opened a dialog at all.
    const asked = await screen.findByText(
      /only the connection between them changes/i,
    );
    expect(asked.textContent).toContain("Employer GmbH");
    // Nothing was written by the press that opened it. The restore route is
    // the only write this surface makes, so its absence from the calls is the
    // whole claim.
    const reached = fetchMock.mock.calls.map(([input]) =>
      String(input instanceof Request ? input.url : input),
    );
    expect(reached.some((url) => url.includes("/restore"))).toBe(false);
  });
});
