/** @vitest-environment happy-dom */

import { LocaleProvider } from "@margince/frontend/app";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import BillingDemoScreen from "./screen";

// The billing demo's screen, over a stubbed transport. Every body is a
// fixture, so the unit's Go tests are the other half of the pair.

const ADMIN = {
  seat_type: "full",
  objects: { ext_billing_demo_feed: { read: true, create: true, update: true } },
};
const READER = {
  seat_type: "full",
  objects: { ext_billing_demo_feed: { read: true } },
};

const INVOICE = {
  erp_id: "INV-1001",
  name: "Acme Corp - annual licence",
  currency: "EUR",
  amount_minor: 1200000,
  paid: false,
  synced: true,
  deal_id: "01a1765c-628f-73ae-83ae-72fd75b32030",
  withheld: [],
};

type Handler = (body: unknown) => unknown;

function stubTransport(
  authorization: unknown,
  handlers: Readonly<Record<string, Handler>>,
) {
  const calls: { path: string; method: string; body: unknown }[] = [];
  const fetchStub = async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const json = (value: unknown, status = 200) =>
      new Response(JSON.stringify(value), {
        status,
        headers: { "Content-Type": "application/json" },
      });
    if (url.endsWith("/v1/me")) {
      return json({ user: {}, roles: [], teams: [], authorization });
    }
    const parsed = new URL(url, "http://stub.invalid");
    const path = parsed.pathname.slice("/v1".length);
    const method = input instanceof Request ? input.method : "GET";
    const raw = input instanceof Request ? await input.text() : "";
    calls.push({ path, method, body: raw === "" ? null : JSON.parse(raw) });
    const handler = handlers[`${method} ${path}`];
    if (!handler) {
      return json({ code: "service_unavailable" }, 503);
    }
    return json(handler(raw === "" ? null : JSON.parse(raw)));
  };
  return { calls, fetchStub };
}

function renderScreen() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <BillingDemoScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const PIPELINES = {
  data: [
    {
      id: "p1",
      is_default: true,
      stages: [
        { id: "s-open", semantic: "open" },
        { id: "s-won", semantic: "won" },
      ],
    },
  ],
};
const USERS = {
  data: [
    { id: "u-human" },
    { id: "u-member", service_extension: "billing-demo" },
  ],
};

describe("the billing demo screen", () => {
  it("names the page in one level-1 heading", async () => {
    const { fetchStub } = stubTransport(ADMIN, {
      "GET /ext/billing-demo/status": () => ({ invoices: [INVOICE] }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));
    renderScreen();
    const h1 = await screen.findByRole("heading", { level: 1 });
    expect(h1.textContent).toBe("Billing demo");
  });

  it("connects with the ids it found, so nobody types one", async () => {
    const { calls, fetchStub } = stubTransport(ADMIN, {
      "GET /ext/billing-demo/status": () => ({ invoices: [] }),
      "GET /users": () => USERS,
      "GET /pipelines": () => PIPELINES,
      "PUT /ext/billing-demo/connection": () => ({ connected: true }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));
    renderScreen();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Connect and add sample invoices" }),
    );
    await waitFor(() => {
      const put = calls.find((call) => call.method === "PUT");
      expect(put?.body).toEqual({
        member_id: "u-member",
        pipeline_id: "p1",
        open_stage_id: "s-open",
        won_stage_id: "s-won",
      });
    });
  });

  it("marks an invoice paid", async () => {
    const { calls, fetchStub } = stubTransport(ADMIN, {
      "GET /ext/billing-demo/status": () => ({ invoices: [INVOICE] }),
      "POST /ext/billing-demo/invoice-update": () => ({ updated: true }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));
    renderScreen();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Mark paid" }));
    await waitFor(() => {
      const post = calls.find((call) => call.method === "POST");
      expect(post?.body).toEqual({ erp_id: "INV-1001", paid: true });
    });
  });

  it("sends a typed amount in minor units", async () => {
    const { calls, fetchStub } = stubTransport(ADMIN, {
      "GET /ext/billing-demo/status": () => ({ invoices: [INVOICE] }),
      "POST /ext/billing-demo/invoice-update": () => ({ updated: true }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));
    renderScreen();
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("New amount for INV-1001"),
      "10000.5",
    );
    await user.click(screen.getByRole("button", { name: "Change amount" }));
    await waitFor(() => {
      const post = calls.find((call) => call.method === "POST");
      expect(post?.body).toEqual({ erp_id: "INV-1001", amount_minor: 1000050 });
    });
  });

  it("says when the amount was kept for a member's edit", async () => {
    const { fetchStub } = stubTransport(ADMIN, {
      "GET /ext/billing-demo/status": () => ({
        invoices: [{ ...INVOICE, withheld: ["amount_minor", "currency"] }],
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));
    renderScreen();
    expect(
      await screen.findByText(/kept, because a member edited it/),
    ).toBeTruthy();
  });

  it("offers no control to a seat that may only read", async () => {
    const { fetchStub } = stubTransport(READER, {
      "GET /ext/billing-demo/status": () => ({ invoices: [INVOICE] }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));
    renderScreen();
    await screen.findByText(/INV-1001/);
    expect(screen.queryByRole("button", { name: "Mark paid" })).toBeNull();
  });
});
