/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { DetailsGrid } from "./companyraildetails";

// The address is six rows of one fact, and a crawled record fills none of them:
// a team page names no registered address. Drawn inline, the panel opened with
// six consecutive invitations to type and pushed the account's actual facts
// below the fold — so the parts sit behind a disclosure that is closed exactly
// when there is nothing in them.
//
// Both cases assert on the browser's own state (`details.open`, and jest-dom's
// `toBeVisible`, which is the one matcher that knows a closed `<details>` hides
// its content): the six labels stay in the DOM either way, and dom-testing-
// library's role queries return them whether the disclosure is open or shut, so
// an absence assertion phrased as a missing node would pass on markup that
// shows the reader all six.

type Organization = components["schemas"]["Organization"];
type ProfileField = components["schemas"]["CompanyProfileField"];

// A COMPLETE Organization, not a cast one: a fixture asserted into the contract
// type can drop a required field and still compile, so the test would go on
// passing after the wire shape moved under it.
const ORG: Organization = {
  id: "o-1",
  // The server answers this per row; a fixture without it reads as NOT
  // writable, which is the correct fail-closed default and would strip the
  // edit affordances these tests are about.
  writable: true,
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-1",
  industry: "Automotive",
  captured_by: "human:u-author",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// The same record with one address part filled — the state that keeps the
// disclosure open. Typed, so a part name the wire stops carrying fails here
// rather than quietly asserting on a field the grid no longer reads.
const ORG_WITH_CITY: Organization = { ...ORG, address: { city: "Berlin" } };

// The six part labels, in the order the grid draws them.
const PART_LABELS = [
  "Street and number",
  "Address line 2",
  "Postal code",
  "City",
  "State / region",
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
        return json(meFixture({ allow: { organization: ["read", "update"] } }));
      }
      if (pathname.endsWith("/profile-fields")) {
        return json({ data: profileFields });
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
// same reason ORG is: a fixture missing a required field still compiles.
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

function renderGrid(organization: Organization) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DetailsGrid organization={organization} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Settles the grid on the grant read: until /me lands every row is read-only,
// which is a different rendering of the same record and not the one under test.
// The industry row is the anchor because it sits outside the disclosure and
// carries a value in every fixture here.
async function renderSettledGrid(
  organization: Organization,
  profileFields: readonly ProfileField[] = [],
  answers: { patch?: () => Response; vatCheck?: () => Response } = {},
) {
  const calls = stub(profileFields, answers);
  renderGrid(organization);
  await screen.findByRole("button", { name: "Change Industry" });
  return calls;
}

// The two legal-identity fields that live only in the evidence sidecar. Before
// these rows a rep could read a VAT number the crawl had found and could not
// state one it had missed — and most sites print no imprint at all, so the
// field a person most often knows was the field they could never record.
describe("the legal identity a person can state", () => {
  it("invites a VAT number and a registry address on a record carrying neither", async () => {
    await renderSettledGrid(ORG);

    // Visible, not merely present: these sit in the identity grid beside the
    // legal name, never inside the address disclosure, which is closed here.
    expect(screen.getByText("Add VAT ID")).toBeVisible();
    expect(screen.getByText("Add registered address")).toBeVisible();
    expect(screen.getByText("Register / VAT ID")).toBeVisible();
    expect(screen.getByText("Registered address")).toBeVisible();
  });

  // The defect the whole move is about: this verdict used to live on another
  // tab behind a collapsed section, so a reader looking straight at the number
  // never met it. A test that only proved the mark RENDERS would have passed
  // for the old surface too — what has to be held is that it renders HERE.
  it("carries the register's verdict beside the number it answers for", async () => {
    await renderSettledGrid(
      ORG,
      [profileField("register_vat", "DE811907980")],
      {
        vatCheck: () =>
          json({
            organization_id: "o-1",
            vat_number: "DE811907980",
            status: "valid",
            checked_at: "2026-08-14T09:12:00Z",
          }),
      },
    );

    // In the same row as the number, which is the entire point.
    const mark = await screen.findByRole("button", { name: "VAT ID: Valid" });
    const row = screen.getByText("DE811907980").closest(".fieldgrid-value");
    expect(row).toContainElement(mark);
  });

  it("carries no verdict on the registry address, which no register answers for", async () => {
    // BOTH sidecar fields, and the VAT one answered: the mark draws nothing
    // until its read settles, so a fixture with only the address would find no
    // mark whether or not the field guard exists. Waiting for the VAT mark to
    // appear is what makes the address's own absence a settled fact.
    await renderSettledGrid(
      ORG,
      [
        profileField("register_vat", "DE811907980"),
        profileField("registered_address", "Kaiserdamm 1, 14057 Berlin"),
      ],
      {
        vatCheck: () =>
          json({
            organization_id: "o-1",
            vat_number: "DE811907980",
            status: "valid",
            checked_at: "2026-08-14T09:12:00Z",
          }),
      },
    );
    await screen.findByRole("button", { name: "VAT ID: Valid" });

    // Exactly one mark on the panel, and it is not the address's.
    expect(screen.getAllByRole("button", { name: /VAT ID:/ })).toHaveLength(1);
    const addressRow = screen
      .getByText("Kaiserdamm 1, 14057 Berlin")
      .closest(".fieldgrid-value");
    expect(addressRow?.querySelector('[class*="vatmark"]')).toBeNull();
  });

  it("reads back the values the crawl already found", async () => {
    await renderSettledGrid(ORG, [
      profileField("register_vat", "DE811907980"),
      profileField("registered_address", "Kaiserdamm 1, 14057 Berlin"),
    ]);

    expect(await screen.findByText("DE811907980")).toBeVisible();
    expect(screen.getByText("Kaiserdamm 1, 14057 Berlin")).toBeVisible();
    expect(screen.queryByText("Add VAT ID")).toBeNull();
  });

  it("states a typed VAT number through the profile-field correction", async () => {
    const user = userEvent.setup();
    const calls = await renderSettledGrid(ORG);

    await user.click(
      screen.getByRole("button", { name: "Change Register / VAT ID" }),
    );
    await user.type(screen.getByLabelText("Register / VAT ID"), "DE811907980");
    await user.keyboard("{Enter}");

    // The endpoint matters as much as the value: this is the write that queues
    // the VAT consultation, and PATCH /organizations/{id} would not.
    const written = await waitFor(() => {
      const call = calls.find((one) => one.method === "PATCH");
      expect(call).toBeDefined();
      return call;
    });
    expect(written?.pathname).toBe(
      "/v1/organizations/o-1/profile-fields/register_vat",
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
    const calls = await renderSettledGrid(ORG, [
      profileField("register_vat", "DE111111111"),
    ]);
    await screen.findByText("DE111111111");

    await user.click(
      screen.getByRole("button", { name: "Change Register / VAT ID" }),
    );
    await user.clear(screen.getByLabelText("Register / VAT ID"));
    await user.type(screen.getByLabelText("Register / VAT ID"), "DE811907980");
    await user.keyboard("{Enter}");

    // Two people correcting the same number: unpinned, the second silently
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
      ORG,
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

    await user.click(
      screen.getByRole("button", { name: "Change Register / VAT ID" }),
    );
    await user.clear(screen.getByLabelText("Register / VAT ID"));
    await user.keyboard("{Enter}");

    expect(
      await screen.findByText("value must be at least 1 character"),
    ).toBeVisible();
    // The refused write left the claim standing, so the draft is still there to
    // correct rather than the row having gone blank under the reader.
    expect(screen.getByLabelText("Register / VAT ID")).toBeVisible();
  });
});

describe("the postal address, behind one line until it has something in it", () => {
  it("holds the six parts behind one line that invites the first of them", async () => {
    await renderSettledGrid(ORG);

    expect(screen.getByText("Add an address")).toBeVisible();
    expect(document.querySelector("details")?.open).toBe(false);
    for (const label of PART_LABELS) {
      expect(screen.getByText(label)).not.toBeVisible();
    }
    // The regression itself: six "Add …" pressables stacked above the facts a
    // reader came for.
    expect(screen.getByText("Add street and number")).not.toBeVisible();
    // What the panel is for is still on screen, unmoved.
    expect(screen.getByText("Automotive")).toBeVisible();
  });

  it("opens on a half-filled address and reads the part that is set", async () => {
    await renderSettledGrid(ORG_WITH_CITY);

    expect(document.querySelector("details")?.open).toBe(true);
    expect(screen.getByText("Address")).toBeVisible();
    expect(screen.queryByText("Add an address")).toBeNull();
    expect(screen.getByText("City")).toBeVisible();
    expect(screen.getByText("Berlin")).toBeVisible();
    // Open means all six, so the five still-empty parts keep inviting a value
    // exactly as they did before the collapse existed.
    for (const label of PART_LABELS) {
      expect(screen.getByText(label)).toBeVisible();
    }
  });
});
