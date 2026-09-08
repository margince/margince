/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { LicenseCard } from "./license";

// Settings → License: what the license grants and how much of it is used.
//
// The three readings this screen must keep apart, because collapsing any two of
// them tells an admin something untrue:
//
//   a seat cap        the meter reads used against granted
//   no seat cap       a license that limits nothing — no meter, and no "0"
//   over the cap      reported, with the workspace still working
//
// The second is the one a naive client gets wrong: `seats_granted` is absent
// rather than zero, and a screen rendering it as 0 would say the license permits
// nobody AND that every seat is over the limit.

type Entitlement = {
  state: "valid" | "absent" | "rejected";
  seats_used: number;
  over_limit: boolean;
  checked_at: string;
  seats_granted?: number;
  license?: {
    id: string;
    subject: string;
    expiry: string;
    in_grace: boolean;
    renewal_due: boolean;
    company?: string;
    contact_name?: string;
    contact_email?: string;
  };
};

// A licensee with every claim, a year from expiry. Tests that care about the
// seat meter take it as-is; tests about the licensee vary one field.
const HOLDER = {
  id: "0199c4f2-1d6e-7a41-9f0b-7b2a2c1d5e30",
  subject: "acme-prod",
  company: "Acme GmbH",
  contact_name: "Ada Lovelace",
  contact_email: "ada@acme.example",
  expiry: "2027-08-14T09:00:00Z",
  in_grace: false,
  renewal_due: false,
};

function backendFor(entitlement: Entitlement) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(req.url, "http://localhost");
    // The card asks which half this reader may have before it asks for either,
    // so /me is now part of its own read rather than ambient context.
    if (url.pathname.endsWith("/v1/me")) {
      return new Response(
        JSON.stringify(meFixture({ allow: { license: ["read"] } })),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    if (url.pathname.endsWith("/installation/license")) {
      return new Response(JSON.stringify(entitlement), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
  });
}

function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const checkedAt = "2026-08-14T09:00:00Z";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("LicenseCard", () => {
  it("reads used against granted when the license caps seats", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 9,
        seats_granted: 10,
        over_limit: false,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    const meter = await waitFor(() => screen.getByRole("meter"));
    expect(meter.getAttribute("aria-valuenow")).toBe("9");
    expect(meter.getAttribute("aria-valuemax")).toBe("10");
    // A role="meter" takes no accessible name from the terms beside it, so the
    // reading has to be IN the name or a screen reader gets a bare number.
    expect(meter.getAttribute("aria-label")).toContain("9");
    expect(meter.getAttribute("aria-label")).toContain("10");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // The seat reading is ONE row, and what counts as a seat is that row's
  // DESCRIPTION rather than a note under the figures. Which order it is in
  // matters: the rule excludes read-only seats and counts agents, so a reader
  // who meets it after the numbers has already taken them for something else.
  it("puts what counts as a seat above the figures it qualifies", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 9,
        seats_granted: 10,
        over_limit: false,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    const rule = await waitFor(() => screen.getByText(/Read-only seats/));
    const row = rule.closest(".settingrow");
    if (!row) {
      throw new Error("the seat rule is not a settings row's description");
    }
    // One row holds the label, the rule, both figures and the bar: the whole
    // comparison, which is what makes it one reading.
    expect(row.textContent).toContain("Seats");
    expect(row.querySelector('[role="meter"]')).not.toBeNull();
    expect(
      rule.compareDocumentPosition(screen.getByRole("meter")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("says a license with no seat count limits nothing, and draws no meter", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 40,
        over_limit: false,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    expect(await waitFor(() => screen.getByText("No limit"))).toBeTruthy();
    // No meter, because there is no maximum to draw one against: a bar filled
    // against a limit nobody set would invent the limit.
    expect(screen.queryByRole("meter")).toBeNull();
    // And the count itself is still shown — the seats are known, only the cap
    // is not.
    expect(screen.getByText("40")).toBeTruthy();
    // Never rendered as a cap of zero, which would read as "permits nobody".
    expect(screen.queryByText("0")).toBeNull();
  });

  it("says an unlicensed installation is unlicensed rather than out of seats", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "absent",
        seats_used: 12,
        over_limit: false,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    expect(
      await waitFor(() => screen.getByText("No license configured")),
    ).toBeTruthy();
    expect(screen.queryByRole("meter")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // This card is the one place the licence gap is stated. The orb used to carry
  // it and no longer does — an unlicensed installation wore permanent amber, so
  // the colour stopped meaning "a fault that can wait" — and a sub-line above a
  // seat meter that reads fine is not somewhere an operator looks. These two
  // cases hold that the fact is still said, and that the two absences are not
  // said as one thing.
  it("says an installation with no license has none, and what that costs", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "absent",
        seats_used: 12,
        over_limit: false,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    expect(
      await waitFor(() => screen.getByText("This installation has no license")),
    ).toBeTruthy();
    expect(screen.getByText(/nothing is capped/)).toBeTruthy();
    // A standing condition does not interrupt: over-the-grant owns the only
    // alert on this screen.
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("keeps a refused license apart from one that was never configured", async () => {
    // Asked and told no is a live fault with a repair behind it; never
    // configured is a standing condition. Both used to print "No license
    // configured", which told an operator with a token to fix that there was
    // nothing to fix.
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "rejected",
        seats_used: 12,
        over_limit: false,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    expect(
      await waitFor(() =>
        screen.getByText("This installation's license was refused"),
      ),
    ).toBeTruthy();
    expect(screen.getByText("License refused")).toBeTruthy();
    expect(screen.queryByText("No license configured")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("interrupts with the numbers and the way back when the installation is over its entitlement", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 11,
        seats_granted: 10,
        over_limit: true,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    // `alert` rather than a quiet notice: being past the entitlement is
    // something the admin has to act on, not notice eventually.
    const alert = await waitFor(() => screen.getByRole("alert"));
    expect(alert.textContent).toContain("11");
    expect(alert.textContent).toContain("10");
    // The copy has to say both halves, because an admin acts on the difference:
    // nobody currently working loses anything (P7), and the next invitation is
    // the thing that will not go through.
    expect(alert.textContent).toMatch(/nobody loses access/i);
    expect(alert.textContent).toMatch(/no new member can be invited/i);
    // The meter still reads, clamped by the component rather than misreporting:
    // the value is the truth and the maximum is the entitlement.
    const meter = screen.getByRole("meter");
    expect(meter.getAttribute("aria-valuenow")).toBe("11");
    expect(meter.getAttribute("aria-valuemax")).toBe("10");
  });
});

describe("the capacity half, for a reader who may not see the commercial one", () => {
  // `seat_usage` shipped so management could plan headcount without being handed
  // what the installation pays. The card reads the entitlement for a `license`
  // holder and falls back to `/installation/seat-usage` for a `seat_usage` one.
  //
  // Both cases assert which endpoint was ASKED, not only what was drawn. The
  // wrong one is not a rendering difference: for a `seat_usage` holder the
  // entitlement 403s, and for a `license` holder the capacity call is a second
  // request for a number the first already carried.
  function twoHalfBackend(
    allow: NonNullable<Parameters<typeof meFixture>[0]>["allow"],
    asked: string[],
  ) {
    return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url, "http://localhost");
      asked.push(url.pathname);
      const body = url.pathname.endsWith("/v1/me")
        ? meFixture({ roles: ["management"], allow })
        : url.pathname.endsWith("/installation/seat-usage")
          ? { seats_used: 7 }
          : {
              state: "valid",
              seats_used: 7,
              over_limit: false,
              checked_at: checkedAt,
            };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
  }

  it("reads capacity for a seat_usage holder, and never asks for the entitlement", async () => {
    const asked: string[] = [];
    vi.stubGlobal("fetch", twoHalfBackend({ seat_usage: ["read"] }, asked));
    render(<LicenseCard />);

    expect(await screen.findByText("7")).toBeTruthy();
    expect(asked).toContain("/v1/installation/seat-usage");
    expect(asked).not.toContain("/v1/installation/license");
  });

  it("reads the entitlement for a license holder, and never asks for capacity", async () => {
    // The half that a naive `!canReadLicense` gets wrong: every capability
    // predicate reads false while /me is in flight, so a fallback keyed on it
    // alone fires the capacity request for a licence holder on every load,
    // before the grant has resolved.
    const asked: string[] = [];
    vi.stubGlobal("fetch", twoHalfBackend({ license: ["read"] }, asked));
    render(<LicenseCard />);

    await waitFor(() => expect(asked).toContain("/v1/installation/license"));
    expect(asked).not.toContain("/v1/installation/seat-usage");
  });

  // The third state of the snapshot, which is neither of the two above: /me
  // FAILED. `isPending` goes false with no grants to read, so a branch keyed on
  // `!canReadLicense` alone is true for a licence holder exactly as it is for a
  // capacity reader — and it would pick capacity permanently, offering a retry
  // that retries seats rather than the snapshot that actually failed.
  it("asks for neither half when the access snapshot itself failed", async () => {
    const asked: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        const url = new URL(req.url, "http://localhost");
        asked.push(url.pathname);
        if (url.pathname.endsWith("/v1/me")) {
          return new Response(JSON.stringify({ title: "upstream" }), {
            status: 500,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(JSON.stringify({ seats_used: 7 }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<LicenseCard />);

    // Waited on the FAILURE having been rendered, not merely on the request
    // having been made: asserting an absence a beat after mount passes before
    // the wrong branch has had a chance to fire, which is the vacuous shape
    // this whole file is careful about. The retry button is the /me gate's own
    // and only appears once the query has actually errored.
    expect(
      await screen.findByRole("button", { name: /try again|retry/i }),
    ).toBeTruthy();
    // Only then: no capacity reading claimed, and no request spent guessing.
    expect(asked).not.toContain("/v1/installation/seat-usage");
    expect(asked).not.toContain("/v1/installation/license");
    expect(screen.queryByText("7")).toBeNull();
  });
});

describe("the licensee", () => {
  it("names the holder, the installation and the support reference", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 9,
        seats_granted: 10,
        over_limit: false,
        checked_at: checkedAt,
        license: HOLDER,
      }),
    );
    render(<LicenseCard />);

    expect(await waitFor(() => screen.getByText("Acme GmbH"))).toBeTruthy();
    expect(screen.getByText(/Ada Lovelace/)).toBeTruthy();
    expect(screen.getByText(/ada@acme.example/)).toBeTruthy();
    expect(screen.getByText("acme-prod")).toBeTruthy();
    expect(screen.getByText(HOLDER.id)).toBeTruthy();
  });

  // A license issued before those claims existed verifies like any other. Its
  // rows are absent, not empty: an empty row says something is missing from
  // THIS license rather than from the vocabulary it was issued under.
  it("renders no row for a claim the license does not carry", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 9,
        seats_granted: 10,
        over_limit: false,
        checked_at: checkedAt,
        license: {
          id: HOLDER.id,
          subject: HOLDER.subject,
          expiry: HOLDER.expiry,
          in_grace: false,
          renewal_due: false,
        },
      }),
    );
    render(<LicenseCard />);

    expect(await waitFor(() => screen.getByText("acme-prod"))).toBeTruthy();
    expect(screen.queryByText("Company")).toBeNull();
    expect(screen.queryByText("Contact")).toBeNull();
  });

  it("asks for a renewal without interrupting when expiry is near", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 9,
        seats_granted: 10,
        over_limit: false,
        checked_at: checkedAt,
        license: { ...HOLDER, renewal_due: true },
      }),
    );
    render(<LicenseCard />);

    expect(
      await waitFor(() => screen.getByText("This license needs a renewal")),
    ).toBeTruthy();
    // Amber, not an alert: nothing has gone wrong yet.
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // Past expiry and still accepted. This one interrupts, because the
  // installation will stop working.
  it("interrupts when the license runs on its grace period", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "valid",
        seats_used: 9,
        seats_granted: 10,
        over_limit: false,
        checked_at: checkedAt,
        license: { ...HOLDER, in_grace: true, renewal_due: true },
      }),
    );
    render(<LicenseCard />);

    const alert = await waitFor(() => screen.getByRole("alert"));
    expect(alert.textContent).toMatch(/expired/i);
    expect(alert.textContent).toMatch(/still works/i);
    // One notice, not two: the grace state supersedes the renewal warning.
    expect(screen.queryByText("This license needs a renewal")).toBeNull();
  });

  it("shows no licensee card for an unlicensed installation", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({
        state: "absent",
        seats_used: 12,
        over_limit: false,
        checked_at: checkedAt,
      }),
    );
    render(<LicenseCard />);

    expect(
      await waitFor(() => screen.getByText("No license configured")),
    ).toBeTruthy();
    expect(screen.queryByText("Licensed to")).toBeNull();
  });
});
