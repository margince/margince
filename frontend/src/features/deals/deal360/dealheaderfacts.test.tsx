// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FieldGrid } from "../../../design-system/fieldgrid";
import { UTC_ZONE } from "../../../format/timezone";
import { type Locale, LocaleProvider, translate } from "../../../i18n";
import { en } from "../../../i18n/en";
import { closeWords, DealRecordRows, DealSubtitle } from "./dealheaderfacts";

// The facts a rep checks first, on the record's head: value, stage, owner —
// the same three questions DealIdentityLine's dot-joined sentence answered,
// now as the named cells every other record's head carries.
//
// The owner was once on no part of the page — not the header, not the rail,
// not the readings — so "whose deal is this" could only be answered by
// opening Edit.

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// The owner and the partner/company references each resolve through their
// own read (the roster, EntityRef's name lookup). None of these tests cares
// about a resolved NAME — each checks the fallback, the mask, or a word
// beside the reference — so every route answers the same empty, honest page,
// on every test rather than the ones that ask for a reference by hand.
beforeEach(() => {
  vi.stubGlobal("fetch", () => Promise.resolve(jsonResponse({ data: [] })));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(node: React.ReactNode, locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const stages = [
  { id: "st-1", name: "Qualified" },
  { id: "st-2", name: "Proposal" },
];

describe("the details rows say where a deal stands and how it closed", () => {
  it("names the stage rather than showing its id", () => {
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{ amount_minor: 6_400_000, currency: "EUR", stage_id: "st-1" }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(screen.getByText("Qualified")).toBeInTheDocument();
    expect(screen.queryByText("st-1")).not.toBeInTheDocument();
  });

  it("says how a won deal was won when no contract carried it", () => {
    // The server treats this answer as load-bearing — the whole justification
    // for letting the deal close without paperwork is that the gap becomes
    // countable — and nothing read it back, so the rep who answered could not
    // check their own answer.
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            status: "won",
            won_without_contract_reason: "purchase_order",
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(
      screen.getByText(en["deals.winReasonPurchaseOrder"]),
    ).toBeInTheDocument();
  });

  it("prints the words a contact wrote rather than the category they chose", () => {
    // `other` is the only reason carrying a detail, and the detail is the only
    // part of this answer somebody typed. "Something else: renewed on a
    // handshake" says the category twice and buries it.
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            status: "won",
            won_without_contract_reason: "other",
            won_without_contract_detail: "Renewed on a handshake at the fair",
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(
      screen.getByText("Renewed on a handshake at the fair"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(en["deals.winReasonOther"]),
    ).not.toBeInTheDocument();
  });

  it("keeps a long hand-written reason readable rather than only hoverable", () => {
    // The strip is a row of SHORT cells, so this one is bounded — but
    // VISUALLY, with the whole string in the DOM. A tooltip wants a mouse, is
    // ignored by most screen readers, and never appears for a keyboard or
    // touch reader. The contact most likely to look is the one checking the
    // words they just typed.
    const long =
      `Renewed on a handshake at the trade fair ${"and again ".repeat(20)}`.trim();
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            status: "won",
            won_without_contract_reason: "other",
            won_without_contract_detail: long,
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(screen.getByText(long)).toBeInTheDocument();
    expect(screen.getByText(long)).toHaveClass("deal-close-detail");
    expect(screen.getByText("Qualified")).toBeInTheDocument();
  });

  // The bound is on WIDTH, never on content — asserted against the
  // stylesheet, because jsdom applies none and a rendered tree cannot tell a
  // wrapped value from a clipped one.
  it("bounds the won-reason detail's width and never its content", () => {
    const css = readFileSync(
      join(resolve(__dirname, ".."), "dealstatus.css"),
      "utf8",
    );
    const rule = /\.deal-close-detail\s*\{([^}]*)\}/.exec(css);
    expect(
      rule,
      ".deal-close-detail is gone from dealstatus.css",
    ).not.toBeNull();
    const body = rule?.[1] ?? "";

    expect(body).toMatch(/max-width:/);
    for (const clip of [
      /overflow\s*:\s*hidden/,
      /text-overflow\s*:/,
      /white-space\s*:\s*nowrap/,
      /line-clamp\s*:/,
    ]) {
      expect(
        body,
        `.deal-close-detail clips its content (${clip.source}); the value is what a controller is shown`,
      ).not.toMatch(clip);
    }
  });

  it("states why a lost deal was lost", () => {
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            status: "lost",
            lost_reason: "Budget frozen until next year",
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(screen.getByText(en["deals.lostReason"])).toBeInTheDocument();
    expect(
      screen.getByText("Budget frozen until next year"),
    ).toBeInTheDocument();
  });

  it("asks nothing about a loss on a deal that is not lost", () => {
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            status: "open",
            lost_reason: "stale",
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(screen.queryByText(en["deals.lostReason"])).not.toBeInTheDocument();
  });

  it("says nothing about paperwork on a won deal a contract carried", () => {
    // The ordinary case, and it needs no cell. A cell on every won deal would
    // bury the ones that need reading — the whole point of showing this at all.
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            status: "won",
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    for (const key of [
      "deals.winReasonPurchaseOrder",
      "deals.winReasonVerbal",
      "deals.winReasonRenewalByEmail",
      "deals.winReasonImported",
      "deals.winReasonOther",
    ] as const) {
      expect(screen.queryByText(en[key])).not.toBeInTheDocument();
    }
  });

  it("never prints a stage id the pipeline cannot name", () => {
    // The case a null stage_id CANNOT test: an id that is present and does not
    // resolve. A deal read before its pipeline finishes loading has stages
    // empty, which reaches the fallback with a real uuid in hand, and printing
    // it puts a machine identifier where a reader expects "Qualified".
    const foreign = "01a02be8-c8d5-7d9b-bb60-a5e1ad68533c";
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{ amount_minor: 1000, currency: "EUR", stage_id: foreign }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText(foreign)).not.toBeInTheDocument();
  });

  // The stored source is a key, so the popover says it in the reader's words.
  it("names how the record reached Margince in words, behind the provenance tag", async () => {
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            source: "manual",
          }}
          stages={stages}
          locale="de"
        />
      </FieldGrid>,
      "de",
    );
    await userEvent.setup().click(screen.getByRole("button"));
    expect(await screen.findByText("Manuell erstellt")).toBeInTheDocument();
    expect(screen.queryByText("manual")).not.toBeInTheDocument();
  });

  // A deal somebody wrote in the system it was imported from. An import runs as
  // ONE administrator, so `captured_by` names that seat on every row it wrote —
  // true, and useless as a statement about authorship. The author field is the
  // one that knows, and the tag has to prefer it.
  it("names who wrote an imported deal there, not the seat that ran the import", () => {
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            source: "hubspot",
            captured_by: "human:u-importer",
            author: { display_name: "Mutaz Suleiman", via: "HubSpot" },
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );
    expect(
      screen.getByText("Logged in HubSpot by Mutaz Suleiman"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Typed by/)).not.toBeInTheDocument();
  });

  // "Source" is the channel a deal came through, so who recorded it has its own
  // label. The system it was logged in reads as its maker spells it.
  it("labels who recorded the deal apart from its source, naming the system as its maker does", () => {
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            stage_id: "st-1",
            source: "hubspot",
            captured_by: "human:u-importer",
            author: { display_name: "Mutaz Suleiman", via: "hubspot" },
          }}
          stages={stages}
          locale="vi"
        />
      </FieldGrid>,
      "vi",
    );
    // "Nguồn" is Source's word, and "bởi" a passive no reader says.
    expect(screen.getByText("Bên ghi nhận")).toBeInTheDocument();
    expect(screen.queryByText(/Nguồn|bởi/)).not.toBeInTheDocument();
    expect(
      screen.getByText("Mutaz Suleiman đã ghi nhận trong HubSpot"),
    ).toBeInTheDocument();
  });
});

describe("the subtitle names the company beside the deal's name", () => {
  it("draws nothing when the deal names no company", () => {
    const { container } = show(<DealSubtitle deal={{ company_id: null }} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("names the field it may not show rather than a bare dash", () => {
    show(
      <DealSubtitle
        deal={{ company_id: null, masked_fields: ["company_id"] }}
      />,
    );
    expect(screen.getByText("Company")).toBeInTheDocument();
  });
});

describe("the close date says how much it is worth believing", () => {
  const t = (
    key: Parameters<typeof translate>[1],
    params?: Record<string, string>,
  ) => translate("en", key, params);
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-24T00:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });
  const words = (deal: Parameters<typeof closeWords>[0]) =>
    closeWords(deal, "en", UTC_ZONE, t);

  it("marks a date no human confirmed", () => {
    const close = words({
      expected_close_date: "2099-09-30",
      close_date_provisional: true,
    });
    expect(close.parts).toContain(en["deal.strip.close.provisional"]);
    expect(close.warn).toBe(true);
  });

  it("says nothing about confirmation when a human set the date", () => {
    const close = words({
      expected_close_date: "2099-09-30",
      close_date_provisional: false,
    });
    expect(close.parts.join(" ")).toMatch(/in [\d,]+ days/);
    expect(close.parts.join(" ")).not.toMatch(/provisional/);
  });

  it("counts the days a deal is already past, in the reader's own plural", () => {
    // "1 days overdue" is what a catalogue that spells the plural once says
    // on the day a rep is most likely to look.
    const close = words({ expected_close_date: "2026-08-23" });
    expect(close.parts).toContain("1 day overdue");
    expect(close.daysLeft).toBe(-1);
  });

  it("never calls a won or lost deal overdue for a date behind it", () => {
    // A closed deal's date is when it closed.
    for (const status of ["won", "lost"] as const) {
      const close = words({
        expected_close_date: "2026-08-23",
        closed_at: "2026-08-23T12:00:00Z",
        status,
      });
      expect(close.parts.join(" ")).not.toMatch(/overdue/);
      expect(close.daysLeft).toBeUndefined();
      expect(close.date).toMatch(/23/);
    }
  });

  it("reads a closed deal's day from closed_at, not from the forecast", () => {
    const close = words({
      expected_close_date: "2026-03-02",
      closed_at: "2026-08-23T12:00:00Z",
      status: "won",
    });
    expect(close.date).toMatch(/23/);
    expect(close.date).not.toMatch(/\b2\b/);
  });

  it("names the day an imported win closed though it has no forecast date", () => {
    const close = words({ closed_at: "2026-08-23T12:00:00Z", status: "won" });
    expect(close.date).toMatch(/23/);
    expect(close.daysLeft).toBeUndefined();
  });

  it("names no day when nobody has set a date", () => {
    expect(words({}).date).toBeUndefined();
  });
});

describe("a deal an import wrote", () => {
  const imported = (importedFrom: string | null) =>
    show(
      <FieldGrid>
        <DealRecordRows
          deal={{
            amount_minor: 1000,
            currency: "EUR",
            stage_id: "st-1",
            captured_by: "human:admin",
            source: "manual",
            imported_from: importedFrom,
          }}
          stages={stages}
          locale="en"
        />
      </FieldGrid>,
    );

  it("reads as imported from its source, not typed by the administrator", () => {
    imported("crm");

    expect(screen.getByText("Imported from crm")).toBeInTheDocument();
    expect(screen.queryByText(/Typed by/)).toBeNull();
  });

  it("a deal no import wrote carries no imported label", () => {
    imported(null);

    expect(screen.queryByText(/Imported/)).toBeNull();
  });
});
