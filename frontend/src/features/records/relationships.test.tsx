/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { type Locale, LocaleProvider, translate } from "../../i18n";
import { renderWithQueryClient } from "../../testing/queryrender";
import { jsonResponse, stubWithSession } from "../../testing/story-utils";
import {
  AddRelationshipAction,
  counterpartyRef,
  edgeOptions,
  endpointBody,
  kindLabelFor,
  type RelationshipScope,
} from "./relationships";

type Relationship = components["schemas"]["Relationship"];

// The relationship picker's kind→endpoint mapping mirrors the backend
// rel_*_shape CHECK constraints (migration 0007). These pure-function specs
// pin that mapping so the UI can never offer a (scope, kind) it can't satisfy
// — the mismatch that used to reach the server as a "endpoint shape is
// required" 422. Interactive coverage of the picker lives in contacts.test.tsx
// / companies.test.tsx; this file is the invariant itself.

const contactScope: RelationshipScope = { contact_id: "p-1" };
const companyScope: RelationshipScope = { company_id: "o-1" };
const dealScope: RelationshipScope = { deal_id: "d-1" };

function baseRel(over: Partial<Relationship>): Relationship {
  return {
    id: "rel-1",
    kind: "employment",
    is_current_primary: false,
    source: "manual",
    captured_by: "human:u-1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

describe("edgeOptions — creatable kinds per scope", () => {
  it("a contact anchors employment (→company) and deal_stakeholder (→deal), nothing company↔company", () => {
    expect(edgeOptions(contactScope)).toEqual([
      { kind: "employment", entity: "company", field: "company_id" },
      { kind: "deal_stakeholder", entity: "deal", field: "deal_id" },
    ]);
  });

  // A deal had no scope of its own, so a stakeholder was creatable only from
  // the CONTACT's side: adding a champion meant knowing which contact to open
  // first, and a deal nobody had linked from a contact page had no way in.
  it("a deal anchors its stakeholders and nothing else", () => {
    expect(edgeOptions(dealScope)).toEqual([
      { kind: "deal_stakeholder", entity: "contact", field: "contact_id" },
    ]);
  });

  it("a company anchors employment (→contact) and the three company↔company kinds (→counterparty), never deal_stakeholder", () => {
    expect(edgeOptions(companyScope)).toEqual([
      { kind: "employment", entity: "contact", field: "contact_id" },
      {
        kind: "partner_of",
        entity: "company",
        field: "counterparty_company_id",
      },
      {
        kind: "referred_by",
        entity: "company",
        field: "counterparty_company_id",
      },
      {
        kind: "co_sell_with",
        entity: "company",
        field: "counterparty_company_id",
      },
    ]);
    expect(
      edgeOptions(companyScope).some((o) => o.kind === "deal_stakeholder"),
    ).toBe(false);
  });
});

describe("endpointBody — the picked id lands on exactly one field", () => {
  it("maps each field to its own key and no other", () => {
    expect(endpointBody("company_id", "x")).toEqual({
      company_id: "x",
    });
    expect(endpointBody("contact_id", "x")).toEqual({ contact_id: "x" });
    expect(endpointBody("counterparty_company_id", "x")).toEqual({
      counterparty_company_id: "x",
    });
    expect(endpointBody("deal_id", "x")).toEqual({ deal_id: "x" });
  });
});

describe("kindLabelFor — an edge reads the right way round from each end", () => {
  // B was referred by the partner A: company_id is B, counterparty is A.
  const referral = (): Relationship =>
    baseRel({
      kind: "referred_by",
      company_id: "o-b",
      counterparty_company_id: "o-a",
    });

  it("reads as referred by on the referred company's page", () => {
    expect(kindLabelFor(referral(), { company_id: "o-b" })).toBe(
      "rel.kind.referredBy",
    );
  });

  it("reads as referred on the referring partner's page", () => {
    expect(kindLabelFor(referral(), { company_id: "o-a" })).toBe(
      "rel.kind.referred",
    );
  });

  it("keeps every other kind's word from either end", () => {
    const partner = baseRel({
      kind: "partner_of",
      company_id: "o-b",
      counterparty_company_id: "o-a",
    });
    expect(kindLabelFor(partner, { company_id: "o-a" })).toBe(
      "rel.kind.partnerOf",
    );
    expect(kindLabelFor(partner, { company_id: "o-b" })).toBe(
      "rel.kind.partnerOf",
    );
  });
});

describe("counterpartyRef — the other end of an existing edge, typed for EntityRef", () => {
  it("a deal edge resolves to the deal regardless of scope", () => {
    const rel = baseRel({ kind: "deal_stakeholder", deal_id: "d-1" });
    expect(counterpartyRef(rel, contactScope)).toEqual({
      kind: "deal",
      id: "d-1",
    });
  });

  // From the deal, the far end is the CONTACT — the deal is the anchor, not the
  // counterparty, so returning the deal here would point every row at the page
  // the reader is already on.
  it("a stakeholder edge resolves to the contact when the deal is the scope", () => {
    const rel = baseRel({
      kind: "deal_stakeholder",
      deal_id: "d-1",
      contact_id: "p-1",
    });
    expect(counterpartyRef(rel, dealScope)).toEqual({
      kind: "contact",
      id: "p-1",
    });
  });

  it("names no far end for a deal edge that carries no contact", () => {
    expect(counterpartyRef(baseRel({ deal_id: "d-1" }), dealScope)).toBeNull();
  });

  it("a company↔company edge resolves to the counterparty company from the anchor side", () => {
    const rel = baseRel({
      kind: "partner_of",
      company_id: "o-1",
      counterparty_company_id: "o-2",
    });
    expect(counterpartyRef(rel, companyScope)).toEqual({
      kind: "company",
      id: "o-2",
    });
  });

  it("resolves to the OTHER company when the same edge is viewed from the counterparty side", () => {
    // The company list filter matches on either end, so this partner_of edge also
    // appears on o-2's tab; the far end there is the anchor o-1, never o-2.
    const rel = baseRel({
      kind: "partner_of",
      company_id: "o-1",
      counterparty_company_id: "o-2",
    });
    expect(counterpartyRef(rel, { company_id: "o-2" })).toEqual({
      kind: "company",
      id: "o-1",
    });
  });

  it("an employment edge resolves to whichever endpoint the scope is NOT", () => {
    const rel = baseRel({
      kind: "employment",
      contact_id: "p-1",
      company_id: "o-1",
    });
    // From the contact's 360 the counterparty is the company; from the company's, the contact.
    expect(counterpartyRef(rel, contactScope)).toEqual({
      kind: "company",
      id: "o-1",
    });
    expect(counterpartyRef(rel, companyScope)).toEqual({
      kind: "contact",
      id: "p-1",
    });
  });

  it("returns null when no counterparty endpoint is present", () => {
    expect(
      counterpartyRef(baseRel({ contact_id: "p-1" }), contactScope),
    ).toBeNull();
  });
});

describe("adding a relationship", () => {
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("sends the start day picked on the calendar", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true, advanceTimeDelta: 5 });
    vi.setSystemTime(new Date("2026-03-10T09:00:00Z"));
    const posted: unknown[] = [];
    stubWithSession(
      {
        "GET /companies": () =>
          jsonResponse({
            data: [{ id: "o-9", display_name: "Acme Corp" }],
            page: { next_cursor: null },
          }),
        "POST /relationships": (body) => {
          posted.push(body);
          return jsonResponse({ ...baseRel({}), id: "rel-new" }, 201);
        },
      },
      { relationship: ["read", "create"] },
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderWithQueryClient(
      <AddRelationshipAction
        scope={contactScope}
        only={{ kind: "employment", label: "rel.add" }}
      />,
    );

    await user.click(screen.getByTestId("add-relationship"));
    const dialog = within(screen.getByRole("dialog"));
    await user.type(dialog.getByPlaceholderText("Search…"), "acme");
    act(() => {
      vi.advanceTimersByTime(250);
    });
    await user.click(await dialog.findByRole("button", { name: "Acme Corp" }));
    await user.click(dialog.getByRole("button", { name: /^Choose date/ }));
    await user.click(
      await screen.findByRole("button", { name: "Monday, 2 March 2026" }),
    );
    expect(dialog.getByRole("textbox", { name: "Started" })).toHaveProperty(
      "value",
      "02/03/2026",
    );
    await user.click(screen.getByTestId("add-relationship-submit"));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "employment",
      contact_id: "p-1",
      company_id: "o-9",
      started_at: "2026-03-02",
    });
  });

  // The committee's gap offers its missing seat. The reader reads the role in
  // their own words; the server counts the seat by its key.
  async function seatMaiAsChampion(
    retype?: string,
    locale: Locale = "vi",
  ): Promise<unknown[]> {
    vi.useFakeTimers({ shouldAdvanceTime: true, advanceTimeDelta: 5 });
    const posted: unknown[] = [];
    stubWithSession(
      {
        "GET /contacts": () =>
          jsonResponse({
            data: [{ id: "p-9", full_name: "Mai Trần" }],
            page: { next_cursor: null },
          }),
        "POST /relationships": (body) => {
          posted.push(body);
          return jsonResponse({ ...baseRel({}), id: "rel-new" }, 201);
        },
      },
      { relationship: ["read", "create"] },
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderWithQueryClient(
      <LocaleProvider initial={locale}>
        <AddRelationshipAction scope={dealScope} seatRole="champion" />
      </LocaleProvider>,
    );

    await user.click(screen.getByTestId("add-relationship"));
    const dialog = within(screen.getByRole("dialog"));
    const role = dialog.getByRole("textbox", {
      name: translate(locale, "rel.role"),
    });
    expect(role).toHaveProperty(
      "value",
      translate(locale, "co.roleLabel.champion"),
    );
    if (retype !== undefined) {
      await user.clear(role);
      await user.type(role, retype);
    }
    await user.type(
      dialog.getByPlaceholderText(translate(locale, "merge.searchPlaceholder")),
      "mai",
    );
    act(() => {
      vi.advanceTimersByTime(250);
    });
    await user.click(await dialog.findByRole("button", { name: "Mai Trần" }));
    await user.click(screen.getByTestId("add-relationship-submit"));
    await waitFor(() => expect(posted).toHaveLength(1));
    return posted;
  }

  it("opens a missing seat's role in the reader's words and seats it by its key", async () => {
    const posted = await seatMaiAsChampion();
    expect(posted[0]).toMatchObject({ role: "champion" });
  });

  it("seats the role by its key when the reader types its label back", async () => {
    const posted = await seatMaiAsChampion(
      ` ${translate("vi", "co.roleLabel.champion")} `,
    );
    expect(posted[0]).toMatchObject({ role: "champion" });
  });

  it.each(["en", "de", "vi"] as const)(
    "seats another deal role by its key, typed in any case and spacing (%s)",
    async (locale) => {
      const label = translate(locale, "co.roleLabel.economic_buyer");
      const posted = await seatMaiAsChampion(
        label.toUpperCase().replaceAll(" ", "  "),
        locale,
      );
      expect(posted[0]).toMatchObject({ role: "economic_buyer" });
    },
  );

  it("sends a role the reader typed in place of the offered one as typed", async () => {
    const posted = await seatMaiAsChampion("Người ủng hộ nội bộ");
    expect(posted[0]).toMatchObject({ role: "Người ủng hộ nội bộ" });
  });
});
