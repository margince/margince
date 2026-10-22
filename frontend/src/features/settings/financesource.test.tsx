/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { FinanceSourceCard } from "./financesource";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const ADMIN: GrantSpec = {
  integrations: ["read", "create", "update", "delete"],
  company: ["read", "update"],
};

const AVAILABLE = [
  { provider: "ledger_books", description: "Reads the ledger books." },
  { provider: "other_books", description: "Reads the other books." },
];

const CONNECTION = {
  id: "0190a5f0-0000-7000-8000-000000000001",
  provider: "ledger_books",
  status: "active",
  connected_at: "2026-08-01T09:00:00Z",
  last_success_at: "2026-08-02T09:00:00Z",
};

const page = { next_cursor: null, has_more: false };

type Call = { method: string; path: string; q: string | null; body: unknown };

// The backend at the HTTP boundary: answers by method and path from `routes`,
// and records every request so a test can assert what was sent.
function backend(
  routes: Record<string, () => Response | Promise<Response>>,
  allow = ADMIN,
) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url);
      const path = url.pathname.replace(/^\/v1/, "");
      const text = await req.clone().text();
      const body = text ? JSON.parse(text) : null;
      calls.push({
        method: req.method,
        path,
        q: url.searchParams.get("q"),
        body,
      });
      if (path === "/me") return jsonResponse(meFixture({ allow }));
      const handler = routes[`${req.method} ${path}`];
      if (!handler)
        throw new Error(`unexpected request: ${req.method} ${path}`);
      return handler();
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return calls;
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("FinanceSourceCard", () => {
  it("says no source is installed when no unit supplies one", async () => {
    backend({
      "GET /finance/source": () => jsonResponse({ available: [] }),
    });
    render(<FinanceSourceCard />);
    expect(
      await screen.findByText("No accounting source installed"),
    ).toBeTruthy();
    expect(screen.queryByTestId("finance-source-connect")).toBeNull();
  });

  it("says once that a reader without the grants cannot connect, and draws no controls", async () => {
    backend(
      { "GET /finance/source": () => jsonResponse({ available: AVAILABLE }) },
      { integrations: ["read"] },
    );
    render(<FinanceSourceCard />);
    expect(
      await screen.findByText(
        "Read-only: only an administrator can connect or sync an accounting source.",
      ),
    ).toBeTruthy();
    expect(screen.queryByTestId("finance-source-connect")).toBeNull();
  });

  it("connects the chosen source", async () => {
    let connected = false;
    const calls = backend({
      "GET /finance/source": () =>
        jsonResponse(
          connected
            ? { available: AVAILABLE, connection: CONNECTION }
            : { available: AVAILABLE },
        ),
      "PUT /finance/source": () => {
        connected = true;
        return jsonResponse({ available: AVAILABLE, connection: CONNECTION });
      },
      "GET /finance/source/customers": () => jsonResponse({ data: [], page }),
    });
    const user = userEvent.setup();
    render(<FinanceSourceCard />);

    await pickOption(
      user,
      await screen.findByRole("combobox", { name: /Source/ }),
      "other_books",
    );
    await user.click(screen.getByTestId("finance-source-connect"));

    await waitFor(() =>
      expect(calls.find((c) => c.method === "PUT")?.body).toEqual({
        provider: "other_books",
      }),
    );
    expect(await screen.findByText("No customers yet")).toBeTruthy();
  });

  it("shows why a source that cannot be read was refused", async () => {
    backend({
      "GET /finance/source": () => jsonResponse({ available: AVAILABLE }),
      "PUT /finance/source": () =>
        jsonResponse(
          {
            title: "Bad Gateway",
            code: "provider_unusable",
            detail: "ledger_books cannot be read",
          },
          502,
        ),
    });
    const user = userEvent.setup();
    render(<FinanceSourceCard />);
    await user.click(await screen.findByTestId("finance-source-connect"));
    expect(await screen.findByRole("alert")).toBeTruthy();
  });

  it("shows the connection, queues a sync and disconnects after a confirm", async () => {
    const calls = backend({
      "GET /finance/source": () =>
        jsonResponse({ available: AVAILABLE, connection: CONNECTION }),
      "GET /finance/source/customers": () => jsonResponse({ data: [], page }),
      "POST /finance/source/sync": () =>
        jsonResponse({ status: "enqueued" }, 202),
      "DELETE /finance/source": () => new Response(null, { status: 204 }),
    });
    const user = userEvent.setup();
    render(<FinanceSourceCard />);

    expect(await screen.findByText("Active")).toBeTruthy();
    await user.click(await screen.findByTestId("finance-source-sync"));
    await waitFor(() =>
      expect(calls.some((c) => c.method === "POST")).toBe(true),
    );

    await user.click(screen.getByTestId("finance-source-disconnect"));
    expect(calls.some((c) => c.method === "DELETE")).toBe(false);
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Disconnect" }),
    );
    await waitFor(() =>
      expect(calls.some((c) => c.method === "DELETE")).toBe(true),
    );
  });

  it("re-reads the directory once the queued sync records its attempt", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    let synced = false;
    backend({
      "GET /finance/source": () =>
        jsonResponse({
          available: AVAILABLE,
          connection: synced
            ? { ...CONNECTION, last_attempt_at: "2026-08-03T09:00:00Z" }
            : CONNECTION,
        }),
      "GET /finance/source/customers": () =>
        jsonResponse({
          data: synced
            ? [{ external_id: "CUST-A", display_name: "Customer A" }]
            : [],
          page,
        }),
      "POST /finance/source/sync": () =>
        jsonResponse({ status: "enqueued" }, 202),
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<FinanceSourceCard />);

    expect(await screen.findByText("No customers yet")).toBeTruthy();
    await user.click(screen.getByTestId("finance-source-sync"));
    synced = true;
    await vi.advanceTimersByTimeAsync(3_000);
    expect(await screen.findByTestId("finance-customer-CUST-A")).toBeTruthy();
  });

  it("links an unlinked customer, searching for its name first, and unlinks a linked one", async () => {
    const calls = backend({
      "GET /finance/source": () =>
        jsonResponse({ available: AVAILABLE, connection: CONNECTION }),
      "GET /finance/source/customers": () =>
        jsonResponse({
          data: [
            { external_id: "CUST-A", display_name: "Customer A" },
            {
              external_id: "CUST-B",
              display_name: "Customer B",
              company: { id: "co-b", display_name: "Beta GmbH" },
            },
          ],
          page,
        }),
      "GET /companies": () =>
        jsonResponse({
          data: [{ id: "co-a", display_name: "Alpha AG" }],
          page,
        }),
      "PUT /finance/source/customers/CUST-A/link": () =>
        jsonResponse({ external_id: "CUST-A", display_name: "Customer A" }),
      "DELETE /finance/source/customers/CUST-B/link": () =>
        new Response(null, { status: 204 }),
    });
    const user = userEvent.setup();
    render(<FinanceSourceCard />);

    const rowA = await screen.findByTestId("finance-customer-CUST-A");
    await user.click(within(rowA).getByRole("button", { name: "Link" }));
    const dialog = await screen.findByRole("dialog");
    const search = within(dialog).getByRole("searchbox", {
      name: "Find a company",
    });
    expect((search as HTMLInputElement).value).toBe("Customer A");
    await waitFor(() =>
      expect(
        calls.find((c) => c.method === "GET" && c.path === "/companies")?.q,
      ).toBe("Customer A"),
    );
    await user.click(
      await within(dialog).findByRole("button", { name: "Alpha AG" }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Link" }));
    await waitFor(() =>
      expect(calls.find((c) => c.method === "PUT")?.body).toEqual({
        company_id: "co-a",
      }),
    );

    const rowB = screen.getByTestId("finance-customer-CUST-B");
    expect(within(rowB).getByText("Beta GmbH")).toBeTruthy();
    await user.click(within(rowB).getByRole("button", { name: "Unlink" }));
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "DELETE" &&
            c.path === "/finance/source/customers/CUST-B/link",
        ),
      ).toBe(true),
    );
  });

  it("marks only the row being unlinked as busy", async () => {
    let answer: (response: Response) => void = () => {};
    const linked = (id: string, company: string) => ({
      external_id: id,
      display_name: `Customer ${id}`,
      company: { id: `co-${id}`, display_name: company },
    });
    backend({
      "GET /finance/source": () =>
        jsonResponse({ available: AVAILABLE, connection: CONNECTION }),
      "GET /finance/source/customers": () =>
        jsonResponse({
          data: [linked("CUST-B", "Beta GmbH"), linked("CUST-C", "Gamma SA")],
          page,
        }),
      "DELETE /finance/source/customers/CUST-B/link": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    const user = userEvent.setup();
    render(<FinanceSourceCard />);

    const rowB = await screen.findByTestId("finance-customer-CUST-B");
    const rowC = screen.getByTestId("finance-customer-CUST-C");
    const unlinkB = within(rowB).getByRole("button", { name: "Unlink" });
    await user.click(unlinkB);
    await waitFor(() => expect(unlinkB.getAttribute("aria-busy")).toBe("true"));
    expect(
      within(rowC)
        .getByRole("button", { name: "Unlink" })
        .getAttribute("aria-busy"),
    ).toBeNull();
    answer(new Response(null, { status: 204 }));
  });
});
