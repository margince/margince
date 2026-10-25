/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  companyEditFields,
  mapCompanyUpdate,
} from "../../components/companyform";
import { company, jsonResponse } from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { meFixture } from "../../testing/mefixture";
import { CompanyScreen } from "./companies";
import { openProfile, openRecordMenu, render } from "./companies.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("CompanyScreen — edit with If-Match (P-1)", () => {
  it("PATCHes /companies/{id} with If-Match:<version> and only update fields", async () => {
    let patchHeader: string | null = null;
    let patchBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "PATCH") {
        patchHeader = request.headers.get("If-Match");
        patchBody = JSON.parse(await request.text());
        return jsonResponse({
          ...company,
          industry: "manufacturing",
          version: 2,
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Change Industry" }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Manufacturing" }),
    );

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchHeader).toBe("1");
    expect(patchBody).toMatchObject({ industry: "manufacturing" });
    // The company fixture carries no domains, so an edit that doesn't touch them
    // omits the field (untouched) rather than clearing the stored set.
    expect(patchBody).not.toHaveProperty("domains");
  });

  // relationship_types is a replace-set, so the edit modal must prefill it.
  // An unseeded multiselect collects as the empty string, a real empty set.
  // Saving an unrelated field would then retire every type the account has.
  it("preserves the account's relationship types when an unrelated field is edited", async () => {
    let patchBody: unknown = null;
    const partner = {
      ...company,
      lifecycle: "customer",
      relationship_types: ["partner", "supplier"],
    };
    stubFetch(async (url, method, request) => {
      if (method === "PATCH") {
        patchBody = JSON.parse(await request.text());
        return jsonResponse({
          ...partner,
          industry: "manufacturing",
          version: 2,
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(partner);
    });
    render(<CompanyScreen id="o-1" />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Change Industry" }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Manufacturing" }),
    );

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({
      industry: "manufacturing",
    });
    expect(patchBody).not.toHaveProperty("relationship_types");
  });
});

// The edit modal's repeatable domains field replace-sets the company's live
// domains on PATCH. Adding a row from the modal and saving carries a
// `domains[]` body: the fork-owned editable seam over the firmographics card.
describe("CompanyScreen — edit domains round-trip (B7)", () => {
  it("PATCHes domains[] when a domain is added in the edit modal", async () => {
    let patchBody: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "PATCH") {
        patchBody = JSON.parse(await request.text());
        return jsonResponse({ ...company, version: 2 });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);

    // The fixture has no domain yet, so its row is folded in the details.
    await userEvent.click(
      await screen.findByRole("button", { name: /^Show \d+ empty fields$/ }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Domain" }),
    );
    // The modal's own row-adder. The rail's details grid also offers an inline
    // "Add domain" to a seat with the grant, and this test is about the form.
    await userEvent.click(screen.getByRole("button", { name: "Add domain" }));
    await userEvent.type(screen.getByLabelText("Domain *"), "brandt.example");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({
      domains: [{ domain: "brandt.example", is_primary: false }],
    });
  });
});

// The PATCH mapping sends `domains` only when the set changed. Untouched edits
// omit it, and removing every row sends `[]` (clear all). The API tells the two apart.
describe("mapCompanyUpdate — domains change detection (P1)", () => {
  const dom = (domain: string, isPrimary: boolean) => ({
    id: "00000000-0000-0000-0000-000000000000",
    domain,
    is_primary: isPrimary,
    source: "manual",
    captured_by: "human:x",
  });

  it("omits domains when the set is unchanged", () => {
    const body = mapCompanyUpdate(
      { display_name: "X" },
      { domains: [{ domain: "a.test", is_primary: "true" }] },
      [dom("a.test", true)],
    );
    expect(body).not.toHaveProperty("domains");
  });

  it("sends [] when every domain row is removed (clear all)", () => {
    const body = mapCompanyUpdate({ display_name: "X" }, { domains: [] }, [
      dom("a.test", true),
    ]);
    expect(body.domains).toEqual([]);
  });

  it("sends the new set when a domain is added", () => {
    const body = mapCompanyUpdate(
      { display_name: "X" },
      {
        domains: [
          { domain: "a.test", is_primary: "true" },
          { domain: "b.test", is_primary: "" },
        ],
      },
      [dom("a.test", true)],
    );
    expect(body.domains).toEqual([
      { domain: "a.test", is_primary: true },
      { domain: "b.test", is_primary: false },
    ]);
  });
});

// The Firmographics & legal card renders the company's confirmed profile
// fields. A returned field shows with its human label and value. A field the
// read never grounded shows no value, and the card invents none.
describe("CompanyScreen — profile fields card (B5)", () => {
  it("renders a confirmed field's value and draws absent fields as empty rows", async () => {
    stubFetch(async (url) => {
      // The tab's rows are editable, and an edit affordance needs all three
      // write axes: the grant, a full seat, and the record's own `writable`.
      // Left to the catch-all below, /me answers with the company body, with
      // no seat and no grants. Every row then renders read-only, which looks
      // like the values having gone missing.
      if (url.includes("/me")) {
        return jsonResponse(
          meFixture({ allow: { company: ["read", "update"] } }),
        );
      }
      if (url.includes("/profile-fields")) {
        return jsonResponse({
          data: [
            {
              field: "value_proposition",
              value: "Fleet retrofits without downtime",
              source: "site_read",
              captured_by: "agent:capture",
              evidence_snippet: "We retrofit fleets without downtime",
              source_url: "https://brandt.example",
              confidence: 0.9,
              updated_at: "2026-07-01T00:00:00Z",
            },
          ],
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);
    await openProfile();

    await waitFor(() =>
      expect(screen.getByText("Value proposition")).toBeTruthy(),
    );
    expect(screen.getByText("Fleet retrofits without downtime")).toBeTruthy();
    // The row has two controls for two acts. The value is the button that
    // starts an edit; its accessible name invites a change, not the value.
    // The receipt beside it opens the provenance panel. One element cannot be both.
    const buttons = screen.getAllByRole("button");
    const valueControl = buttons.find(
      (el) => el.textContent?.trim() === "Fleet retrofits without downtime",
    );
    expect(valueControl).toBeTruthy();
    // Scoped to the field's own row: the page's overflow menu is also an
    // aria-expanded button. A page-wide search would pass even when the value
    // lost its evidence mark.
    const row = valueControl?.closest(".fieldgrid-value");
    expect(row).toBeTruthy();
    const receipt = Array.from(row?.querySelectorAll("button") ?? []).find(
      (el) => el.getAttribute("aria-expanded") === "false",
    );
    expect(receipt).toBeTruthy();
    expect(screen.queryByText(/^High$|^Medium$|^Low$/)).toBeNull();
    // A field the read never grounded still draws its row, empty. The tab is a
    // form, not a list of what a crawl found: a reader can only add what they
    // can see is missing.
    expect(
      screen.getAllByText("Ideal customer profile").length,
    ).toBeGreaterThan(0);
  });

  it("draws every narrative field as an empty row when nothing has been read", async () => {
    stubFetch(async (url) => {
      if (url.includes("/profile-fields")) {
        return jsonResponse({ data: [] });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);
    await openProfile();

    // An account nobody has read is the case this tab exists for. Every field
    // is present and blank, so the reader can state what they know. A bare
    // "Nothing read yet" would give no way to change that.
    await waitFor(() =>
      expect(screen.getAllByText("Offering").length).toBeGreaterThan(0),
    );
    expect(screen.getAllByText("Sales motion").length).toBeGreaterThan(0);
  });
});

// The facts card groups site-read facts into the four fixed categories,
// omits empty categories, and renders each fact's field → value row.
describe("CompanyScreen — facts card (B6)", () => {
  it("groups facts by category and omits empty categories", async () => {
    stubFetch(async (url) => {
      if (url.endsWith("/facts")) {
        return jsonResponse({
          data: [
            {
              category: "market",
              field: "served_industry",
              value: "Automotive OEMs",
              value_key: "served_industry:automotive-oems",
              source: "site_read",
              captured_by: "agent:capture",
              updated_at: "2026-07-01T00:00:00Z",
            },
            {
              category: "company",
              field: "founded_year",
              value: "1998",
              value_key: "founded_year:1998",
              source: "site_read",
              captured_by: "agent:capture",
              updated_at: "2026-07-01T00:00:00Z",
            },
            {
              category: "offering",
              field: "service",
              value: "Fleet retrofits",
              value_key: "service:fleet-retrofits",
              source: "site_read",
              captured_by: "agent:capture",
              updated_at: "2026-07-01T00:00:00Z",
            },
          ],
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);
    await openProfile();

    await waitFor(() => expect(screen.getByText("Company facts")).toBeTruthy());
    // Scoped to the facts card: the right rail has a Signals card of its own.
    // Which categories the site read produced is a question about this card.
    const factsCard = screen.getByText("Company facts").closest("section");
    if (!factsCard) {
      throw new Error("the facts card has no section wrapper");
    }
    const facts = within(factsCard);
    expect(facts.getByText("Company")).toBeTruthy();
    expect(facts.getByText("Offering")).toBeTruthy();
    expect(facts.getByText("Market")).toBeTruthy();
    expect(facts.getByText("1998")).toBeTruthy();
    expect(facts.getByText("Automotive OEMs")).toBeTruthy();
    expect(facts.getByText("Fleet retrofits")).toBeTruthy();
    // No signal fact was returned, so that subsection is absent.
    expect(facts.queryByText("Signals")).toBeNull();
  });
});

describe("CompanyScreen — archive (P-3)", () => {
  it("opens a confirm, DELETEs /companies/{id} on confirm, and navigates to the list", async () => {
    let deleted = false;
    stubFetch(async (url, method) => {
      if (method === "DELETE" && url.includes("/companies/o-1")) {
        deleted = true;
        return jsonResponse({
          ...company,
          archived_at: "2026-07-13T00:00:00Z",
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    const user = userEvent.setup();
    render(<CompanyScreen id="o-1" />);

    await user.click(await openRecordMenu("archive-record", user));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );

    await waitFor(() => expect(deleted).toBe(true));
    expect(window.location.hash).toBe("#/companies");
  });
});

describe("CompanyScreen — merge into target (P-2)", () => {
  const acme = { ...company, id: "o-2", display_name: "Acme Corp" };

  it("searches, excludes the source row, and merges into the picked target", async () => {
    let mergeBody: unknown = null;
    let mergeHeader: string | null = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/companies/o-1/merge")) {
        mergeHeader = request.headers.get("If-Match");
        mergeBody = JSON.parse(await request.text());
        return jsonResponse({ ...acme, version: 2 });
      }
      if (url.includes("/companies?") && url.includes("q=acme")) {
        return jsonResponse({
          data: [company, acme],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);

    await userEvent.click(await openRecordMenu("merge-record"));
    await userEvent.type(screen.getByPlaceholderText("Search…"), "acme");

    vi.useFakeTimers();
    try {
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }

    const dialog = screen.getByRole("dialog");
    await waitFor(() =>
      expect(within(dialog).getByText("Acme Corp")).toBeTruthy(),
    );
    // The source row must never appear as a mergeable target.
    expect(within(dialog).queryByText("Brandt Automotive GmbH")).toBeNull();

    await userEvent.click(within(dialog).getByText("Acme Corp"));
    await userEvent.click(screen.getByTestId("merge-confirm"));

    await waitFor(() => expect(mergeBody).toBeTruthy());
    expect(mergeBody).toMatchObject({ target_id: "o-2" });
    expect(mergeHeader).toBe("1");
    expect(window.location.hash).toBe("#/companies/o-2");
  });
});

// `UpdateCompanyRequest.owner_id` cannot carry "unassign": on the wire a null
// looks like an omitted field. A blank option would be dropped on save, so the
// save would report success while the old owner stayed responsible.
describe("companyEditFields — the owner select never offers what it cannot save", () => {
  const ownerField = (hasOwner: boolean) =>
    companyEditFields(
      [{ id: "u1", display_name: "Demo Admin" }],
      hasOwner,
      // The identity translator: this suite is about the owner select's
      // required-ness, and a key reads as well as a word for that.
      (key) => key,
      "en",
    ).find((field) => field.key === "owner_id");

  it("is required while the account has an owner, so no blank option renders", () => {
    // An optional select renders a blank option; picking it would send a body
    // the server reads as "leave the owner alone".
    expect(ownerField(true)?.required).toBe(true);
  });

  it("stays optional while the account has no owner at all", () => {
    // There the blank is the truthful current state, not an edit we cannot make.
    expect(ownerField(false)?.required).toBeFalsy();
  });
});
