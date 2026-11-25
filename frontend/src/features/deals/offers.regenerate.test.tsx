/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { OfferScreen } from "./offers";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

const sentOffer = {
  id: "o-1",
  deal_id: "d-1",
  offer_number: "ANG-2026-0007",
  revision: 2,
  status: "sent",
  currency: "EUR",
  buyer_company_id: null,
  valid_until: "2026-08-01",
  intro_text: null,
  terms_text: null,
  net_minor: 100000,
  tax_minor: 19000,
  gross_minor: 119000,
  template_id: null,
  line_items: [],
  source: "manual",
  captured_by: "human:u1",
  version: 3,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** Serves the sent offer, and the next revision for the regenerate POST. */
function serveRegenerate(next: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request ? request.method : (init?.method ?? "GET");
      if (url.includes("/offers/o-1/regenerate") && method === "POST") {
        return json(next, 201);
      }
      if (url.includes("/offers/")) {
        return json(sentOffer);
      }
      return json({ data: [], page: { has_more: false } });
    }),
  );
}

async function regenerate() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <OfferScreen id="o-1" />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await screen.findByText("ANG-2026-0007");
  await userEvent.setup().click(screen.getByTestId("regenerate-offer"));
  await waitFor(() => expect(globalThis.location.hash).toBe("#/offers/o-2"));
}

// The revision asked for was made either way. When no AI line came with it,
// the reader is told the copy is plain.
describe("regenerating a sent offer", () => {
  it("says the copy came without AI changes when no AI line was added", async () => {
    serveRegenerate({
      ...sentOffer,
      id: "o-2",
      status: "draft",
      ai_generated: false,
    });
    await regenerate();
    expect(await screen.findByText("Copied without AI changes.")).toBeTruthy();
  });

  it("says nothing of the kind when AI lines were added", async () => {
    serveRegenerate({
      ...sentOffer,
      id: "o-2",
      status: "draft",
      ai_generated: true,
      ai_disclosure: "Drafted with AI assistance. Review before sending.",
      diff_from_previous: { added: [], removed: [], changed: [] },
    });
    await regenerate();
    expect(screen.queryByText("Copied without AI changes.")).toBeNull();
  });
});
