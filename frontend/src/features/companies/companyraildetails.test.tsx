/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { CompanyRail } from "./companyrail";

// The company's register facts in its details. One is the VAT number the
// design draws in Registration (an "Add…" invitation while empty). The other
// is the postal address, edited as one six-part form. The registered address
// is a Profile tab field (companyprofiledetails.test.tsx), never a row of this
// column.

type Company = components["schemas"]["Company"];
type ProfileField = components["schemas"]["CompanyProfileField"];

// A COMPLETE Company, not a cast one: a fixture asserted into the contract
// type can drop a required field and still compile, so the test would go on
// passing after the wire shape moved under it.
const COMPANY: Company = {
  id: "o-1",
  // The server answers this per row; a fixture without it reads as NOT
  // writable, which is the correct fail-closed default and would strip the
  // edit affordances these tests are about.
  writable: true,
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-1",
  industry: "automotive",
  captured_by: "human:u-author",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// The same record with one address part filled. Typed, so a part name the wire
// stops carrying fails here rather than asserting on a field nobody reads.
const COMPANY_WITH_CITY: Company = { ...COMPANY, address: { city: "Berlin" } };

// The six part labels, in the order the grid draws them.
const PART_LABELS = [
  "Street and number",
  "Address line 2",
  "Postal code",
  "City",
  "State or region",
  "Country",
];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The grant is what makes the rows editable, which is the state the collapse is
// about: a viewer who may not write sees values, and an empty address draws no
// invitation to hide in the first place.
function stub(
  profileFields: readonly ProfileField[] = [],
  answers: { patch?: () => Response; vatCheck?: () => Response } = {},
) {
  const calls: {
    method: string;
    pathname: string;
    body: string;
    ifMatch: string | null;
  }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      calls.push({
        method: request.method,
        pathname,
        // Read here rather than in the assertion: the body stream is consumed
        // once, and a later read would come back empty.
        body: request.method === "GET" ? "" : await request.text(),
        ifMatch: request.headers.get("If-Match"),
      });
      if (pathname.endsWith("/me")) {
        return json(meFixture({ allow: { company: ["read", "update"] } }));
      }
      if (pathname.endsWith("/profile-fields")) {
        return json({ data: profileFields });
      }
      // The tags row reads its own list. Answered as empty rather than left to
      // the fallthrough below, whose rows are colleagues, not tags: a tag row
      // built from a colleague has no assignment date to print.
      if (pathname.endsWith("/tags")) {
        return json({ data: [] });
      }
      // The verdict the VAT mark reads. Answered here rather than left to fall
      // through: the mark draws NOTHING until this settles, so a stub that gave
      // it a list shape would render no mark and a test asserting its absence
      // would pass for the wrong reason.
      if (pathname.endsWith("/vat-check")) {
        return (
          answers.vatCheck?.() ??
          new Response(JSON.stringify({ title: "not found" }), {
            status: 404,
            headers: { "content-type": "application/json" },
          })
        );
      }
      if (request.method === "PATCH") {
        return answers.patch?.() ?? json({});
      }
      return json({
        data: [{ id: "u-1", display_name: "Mira Voss" }],
        page: { has_more: false, next_cursor: null },
      });
    }),
  );
  return calls;
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

// A sidecar claim as the wire carries it. Complete rather than cast, for the
// same reason COMPANY is: a fixture missing a required field still compiles.
function profileField(
  field: ProfileField["field"],
  value: string,
): ProfileField {
  return {
    id: `pf-${field}`,
    field,
    value,
    source: "site_read",
    captured_by: "agent:deepread",
    updated_at: "2026-06-01T08:00:00Z",
    // The version a correction pins. Omitting it would model a row the server
    // does not send and leave the precondition untested.
    version: 4,
  };
}

function renderGrid(company: Company) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyRail company={company} loading={false} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Settles the grid on the grant read: until /me lands every row is read-only,
// which is a different rendering of the same record and not the one under test.
// The industry row is the anchor because it sits outside the disclosure and
// carries a value in every fixture here.
async function renderSettledGrid(
  company: Company,
  profileFields: readonly ProfileField[] = [],
  answers: { patch?: () => Response; vatCheck?: () => Response } = {},
) {
  const calls = stub(profileFields, answers);
  renderGrid(company);
  await screen.findByRole("button", { name: "Change Industry" });
  return calls;
}

// The two legal-identity fields that live only in the evidence sidecar. Before
// these rows a rep could read a VAT number the crawl had found and could not
// state one it had missed — and most sites print no imprint at all, so the
// field a contact most often knows was the field they could never record.
describe("the legal identity a contact can state", () => {
  it("invites a VAT number and draws no registry address row, folded or not", async () => {
    const user = userEvent.setup();
    await renderSettledGrid(COMPANY);

    expect(screen.getByText("VAT ID")).toBeVisible();
    expect(screen.getByText("Add…")).toBeVisible();

    await user.click(screen.getByRole("button", { name: /Show \d+ empty/ }));
    expect(screen.queryByText("Registered address")).toBeNull();
  });

  // The defect the whole move is about: this verdict used to live on another
  // tab behind a collapsed section, so a reader looking straight at the number
  // never met it. A test that only proved the mark RENDERS would have passed
  // for the old surface too — what has to be held is that it renders HERE.
  it("carries the register's verdict beside the number it answers for", async () => {
    await renderSettledGrid(
      COMPANY,
      [profileField("register_vat", "DE811907980")],
      {
        vatCheck: () =>
          json({
            company_id: "o-1",
            vat_number: "DE811907980",
            status: "valid",
            checked_at: "2026-08-14T09:12:00Z",
          }),
      },
    );

    // In the same row as the number, which is the entire point.
    const mark = await screen.findByRole("button", { name: "VAT ID: Valid" });
    const row = screen.getByText("DE811907980").closest(".factlist-value");
    expect(row).toContainElement(mark);
  });

  // The grant is held and the RECORD is not writable — the case the button's
  // old gate could not see. POST /companies/{id}/vat-check runs
  // EnsureWritableLive on this company, so a rep holding `company:
  // update` who does not own the account was offered a check that comes back
  // refused on click.
  //
  // Asserted from the grid rather than from the mark alone: the mark is told
  // whether it may ask, so a unit test of the mark cannot show that the answer
  // it is told is the record's.
  it("offers no register check on a company the reader may not write", async () => {
    const user = userEvent.setup();
    // Not renderSettledGrid: its settle signal is an EDIT affordance, and a
    // grid this reader cannot write draws none. The number itself is the
    // signal here.
    stub([profileField("register_vat", "DE811907980")], {
      vatCheck: () =>
        json({
          company_id: "o-1",
          vat_number: "DE811907980",
          status: "valid",
          checked_at: "2026-08-14T09:12:00Z",
        }),
    });
    renderGrid({ ...COMPANY, writable: false });

    await user.click(
      await screen.findByRole("button", { name: "VAT ID: Valid" }),
    );

    // The verdict is still theirs to read: withholding the ask is not
    // withholding the record.
    expect((await screen.findAllByText("DE811907980"))[0]).toBeVisible();
    expect(screen.queryByRole("button", { name: /Check/ })).toBeNull();
  });

  // A profile-field read that has NOT answered says nothing about whether the
  // field has a row. Treating its empty stand-in as "no row yet" sends the
  // correction with no If-Match, and the server's patch is a locked
  // last-write-wins — so the reader overwrites a value they never saw, with no
  // version conflict to stop them. The row waits for the answer instead.
  it("refuses the edit until the sidecar read has actually answered", async () => {
    let releaseFields: ((value: Response) => void) | undefined;
    const held = new Promise<Response>((resolve) => {
      releaseFields = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = new URL(request.url);
        if (url.pathname.endsWith("/profile-fields")) {
          return held;
        }
        if (url.pathname.endsWith("/me")) {
          return json(meFixture({ allow: { company: ["read", "update"] } }));
        }
        if (url.pathname.endsWith("/vat-check")) {
          return new Response(null, { status: 404 });
        }
        return json({});
      }),
    );
    renderGrid(COMPANY);
    await screen.findByRole("button", { name: "Change Industry" });

    // The industry row edits (it reads the company, which HAS answered);
    // the sidecar rows do not, because their own read has not.
    expect(screen.queryByRole("button", { name: "Change VAT ID" })).toBeNull();

    releaseFields?.(
      json({ data: [profileField("register_vat", "DE811907980")] }),
    );
    expect(
      await screen.findByRole("button", { name: "Change VAT ID" }),
    ).toBeTruthy();
  });

  it("reads back the VAT number the crawl already found", async () => {
    await renderSettledGrid(COMPANY, [
      profileField("register_vat", "DE811907980"),
      profileField("registered_address", "Kaiserdamm 1, 14057 Berlin"),
    ]);

    expect(await screen.findByText("DE811907980")).toBeVisible();
    expect(screen.queryByText("Add…")).toBeNull();
    // The registered address is on the Profile tab, so the column stays the
    // design's three groups however much the crawl found.
    expect(screen.queryByText("Kaiserdamm 1, 14057 Berlin")).toBeNull();
  });

  it("states a typed VAT number through the profile-field correction", async () => {
    const user = userEvent.setup();
    const calls = await renderSettledGrid(COMPANY);

    await user.click(screen.getByRole("button", { name: "Change VAT ID" }));
    await user.type(screen.getByLabelText("VAT ID"), "DE811907980");
    await user.keyboard("{Enter}");

    // The endpoint matters as much as the value: this is the write that queues
    // the VAT consultation, and PATCH /companies/{id} would not.
    const written = await waitFor(() => {
      const call = calls.find((one) => one.method === "PATCH");
      expect(call).toBeDefined();
      return call;
    });
    expect(written?.pathname).toBe(
      "/v1/companies/o-1/profile-fields/register_vat",
    );
    expect(JSON.parse(written?.body ?? "{}")).toEqual({
      value: "DE811907980",
    });
    // Nothing to pin: this write CREATES the row, so there is no earlier state
    // another editor could be overwriting.
    expect(written?.ifMatch).toBeNull();
  });

  it("pins the row when correcting a value somebody already stated", async () => {
    const user = userEvent.setup();
    const calls = await renderSettledGrid(COMPANY, [
      profileField("register_vat", "DE111111111"),
    ]);
    await screen.findByText("DE111111111");

    await user.click(screen.getByRole("button", { name: "Change VAT ID" }));
    await user.clear(screen.getByLabelText("VAT ID"));
    await user.type(screen.getByLabelText("VAT ID"), "DE811907980");
    await user.keyboard("{Enter}");

    // Two contacts correcting the same number: unpinned, the second silently
    // replaces a change its author never saw.
    const written = await waitFor(() => {
      const call = calls.find((one) => one.method === "PATCH");
      expect(call).toBeDefined();
      return call;
    });
    expect(written?.ifMatch).toBe("4");
  });

  // Clearing a stated value is the reader asking to unsay it, and the
  // correction path has no delete — it writes a value or it refuses. The
  // server's own `minLength: 1` is what refuses, so the reader is told rather
  // than left looking at a field that silently kept its old value.
  it("shows the refusal when a stated value is cleared", async () => {
    const user = userEvent.setup();
    await renderSettledGrid(
      COMPANY,
      [profileField("register_vat", "DE811907980")],
      {
        patch: () =>
          new Response(
            JSON.stringify({
              type: "about:blank",
              title: "Validation failed",
              status: 422,
              detail: "value must be at least 1 character",
            }),
            {
              status: 422,
              headers: { "content-type": "application/problem+json" },
            },
          ),
      },
    );
    await screen.findByText("DE811907980");

    await user.click(screen.getByRole("button", { name: "Change VAT ID" }));
    await user.clear(screen.getByLabelText("VAT ID"));
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("value must be at least 1 character"),
    ).toBeVisible();
    // The refused write left the claim standing, so the draft is still there to
    // correct rather than the row having gone blank under the reader.
    expect(screen.getByLabelText("VAT ID")).toBeVisible();
  });

  it.each([
    { is_anchor: true, refetched: true },
    { is_anchor: false, refetched: false },
  ])(
    "reads /me again after a stated field only when is_anchor is $is_anchor",
    async ({ is_anchor, refetched }) => {
      const user = userEvent.setup();
      const calls = await renderSettledGrid({ ...COMPANY, is_anchor });
      const reads = () => calls.filter((one) => one.pathname === "/v1/me");
      const before = reads().length;

      await user.click(screen.getByRole("button", { name: "Change VAT ID" }));
      await user.type(screen.getByLabelText("VAT ID"), "DE1");
      await user.keyboard("{Enter}");
      await waitFor(() => expect(screen.queryByLabelText("VAT ID")).toBeNull());

      expect(reads().length > before).toBe(refetched);
    },
  );
});

describe("the grouped postal address", () => {
  it("keeps the six parts behind one editor even when the address is empty", async () => {
    const user = userEvent.setup();
    await renderSettledGrid(COMPANY);
    await user.click(screen.getByRole("button", { name: /Show \d+ empty/ }));
    await user.click(screen.getByRole("button", { name: "Change Address" }));
    for (const label of PART_LABELS)
      expect(screen.getByLabelText(new RegExp(label))).toBeVisible();
  });
  it("opens with all existing address parts", async () => {
    const user = userEvent.setup();
    await renderSettledGrid(COMPANY_WITH_CITY);
    await user.click(screen.getByRole("button", { name: "Change Address" }));
    expect(screen.getByDisplayValue("Berlin")).toBeVisible();
    for (const label of PART_LABELS)
      expect(screen.getByLabelText(new RegExp(label))).toBeVisible();
  });
});
