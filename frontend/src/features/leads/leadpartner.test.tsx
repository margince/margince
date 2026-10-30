/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { useLeadPartnerRow } from "./leadpartner";
import type { Lead } from "./leads";
import { jsonResponse, lead, render } from "./leads.testkit";

// A lead's "Referred by partner" row: shown once a partner exists, the verb
// that accepts the lead for its partner, and the stamp that replaces it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const WRITER = { readOnly: false, readOnlyReason: undefined };

function Host({ record }: Readonly<{ record: Lead }>) {
  const row = useLeadPartnerRow(record, WRITER, async () => {});
  return <div>{row ? row.value : "no partner row"}</div>;
}

function backend(
  partners: { id: string; display_name: string }[],
  allow: Parameters<typeof meFixture>[0]["allow"],
  calls: string[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      calls.push(`${request.method} ${url.pathname}`);
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (url.pathname.endsWith("/partner-acceptance")) {
        return jsonResponse({
          ...lead,
          partner_accepted_at: "2026-10-30T08:00:00Z",
        });
      }
      return jsonResponse({
        data: partners,
        page: { next_cursor: null, has_more: false },
      });
    }),
  );
}

const PARTNER_MANAGER = {
  lead: ["read", "update"],
  company: ["read"],
  partner: ["read", "update"],
} as const;

it("draws no row while the installation has no partner", async () => {
  backend([], PARTNER_MANAGER);
  render(<Host record={lead} />);
  expect(await screen.findByText("no partner row")).toBeInTheDocument();
});

it("lets a partner manager accept the lead for its partner", async () => {
  const user = userEvent.setup();
  const calls: string[] = [];
  backend(
    [{ id: "c-9", display_name: "Referrer GmbH" }],
    PARTNER_MANAGER,
    calls,
  );
  render(<Host record={{ ...lead, partner_company_id: "c-9" }} />);
  await user.click(
    await screen.findByRole("button", { name: "Accept for partner" }),
  );
  await waitFor(() =>
    expect(calls).toContain("POST /v1/leads/l-1/partner-acceptance"),
  );
});

it("offers no acceptance to a seat that may not change partners", async () => {
  backend([{ id: "c-9", display_name: "Referrer GmbH" }], {
    lead: ["read", "update"],
    company: ["read"],
    partner: ["read"],
  });
  render(<Host record={{ ...lead, partner_company_id: "c-9" }} />);
  expect(await screen.findByText("Referrer GmbH")).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Accept for partner" }),
  ).toBeNull();
});

it("shows the acceptance stamp in place of the verb", async () => {
  backend([{ id: "c-9", display_name: "Referrer GmbH" }], PARTNER_MANAGER);
  render(
    <Host
      record={{
        ...lead,
        partner_company_id: "c-9",
        partner_accepted_at: "2026-10-30T08:00:00Z",
        partner_accepted_by: "human:u-1",
      }}
    />,
  );
  expect(
    await screen.findByText(/Accepted for the partner on/),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Accept for partner" }),
  ).toBeNull();
});
