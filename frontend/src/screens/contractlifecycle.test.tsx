/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import {
  ContractCancelModal,
  ContractRenewModal,
  ContractStatusModal,
  isTerminalContractStatus,
} from "./contractlifecycle";

// margince#3286: the backend chain (Store.Renew, POST /contracts/{id}/renewal)
// was correct and tested; nothing in the app could reach it. These prove the
// modal sends the successor's own terms — inheriting nothing but the
// counterparty, which the server derives — and the version it read, so a
// concurrent edit is refused rather than silently overwritten.

type Contract = components["schemas"]["Contract"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const PREDECESSOR: Contract = {
  id: "c-1",
  organization_id: "o-1",
  title: "Framework agreement 2024",
  source: "manual",
  captured_by: "human:u-1",
  status: "active",
  under_contract: true,
  auto_renew: false,
  value_basis: "annualized_12m",
  version: 3,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
};

function show(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("ContractRenewModal", () => {
  it("posts the successor's own title, basis and the predecessor's version as If-Match", async () => {
    let posted: { body: unknown; ifMatch: string | null; path: string } | null =
      null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(input, init);
        const url = new URL(request.url);
        if (request.method === "POST" && url.pathname.endsWith("/renewal")) {
          posted = {
            body: await request.json(),
            ifMatch: request.headers.get("if-match"),
            path: url.pathname,
          };
          return new Response(
            JSON.stringify({ ...PREDECESSOR, id: "c-2", version: 1 }),
            { status: 201, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response("not found", { status: 404 });
      }),
    );
    const user = userEvent.setup();
    const onClose = vi.fn();
    show(
      <ContractRenewModal
        orgId="o-1"
        contract={PREDECESSOR}
        open
        onClose={onClose}
      />,
    );

    // Prefilled from the predecessor, both editable.
    const title = await screen.findByLabelText(/^Title/);
    expect(title).toHaveValue("Framework agreement 2024");

    await user.click(screen.getByRole("button", { name: "Renew" }));

    await waitFor(() => expect(posted).not.toBeNull());
    const sent = posted as unknown as {
      body: { title?: string; value_basis?: string };
      ifMatch: string | null;
      path: string;
    };
    expect(sent.path).toBe("/v1/contracts/c-1/renewal");
    expect(sent.ifMatch).toBe("3");
    expect(sent.body.title).toBe("Framework agreement 2024");
    expect(sent.body.value_basis).toBe("annualized_12m");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("shows the server's refusal inline rather than closing silently", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              code: "validation_error",
              detail: "a term cannot end before it starts",
            }),
            { status: 422, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );
    const user = userEvent.setup();
    const onClose = vi.fn();
    show(
      <ContractRenewModal
        orgId="o-1"
        contract={PREDECESSOR}
        open
        onClose={onClose}
      />,
    );

    await user.click(await screen.findByRole("button", { name: "Renew" }));

    await screen.findByText(/a term cannot end before it starts/);
    expect(onClose).not.toHaveBeenCalled();
  });
});

// A terminal status has no valid transition out of it other than a same-status
// no-op (refuseInvalidTransition, contract_lifecycle.go), so a row in one of
// these offers neither "change status" nor "cancel" — both would be controls
// that can only refuse, same reasoning #3573/#3700 already hold for the plan's
// write controls.
describe("isTerminalContractStatus", () => {
  it("is true for the three statuses a contract cannot leave", () => {
    expect(isTerminalContractStatus("expired")).toBe(true);
    expect(isTerminalContractStatus("cancelled")).toBe(true);
    expect(isTerminalContractStatus("superseded")).toBe(true);
  });

  it("is false for the two a human can still move", () => {
    expect(isTerminalContractStatus("draft")).toBe(false);
    expect(isTerminalContractStatus("active")).toBe(false);
  });
});

describe("ContractStatusModal", () => {
  it("posts the selected status with the row's version as If-Match", async () => {
    let posted: { body: unknown; ifMatch: string | null; path: string } | null =
      null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(input, init);
        const url = new URL(request.url);
        if (request.method === "POST" && url.pathname.endsWith("/status")) {
          posted = {
            body: await request.json(),
            ifMatch: request.headers.get("if-match"),
            path: url.pathname,
          };
          return new Response(
            JSON.stringify({ ...PREDECESSOR, status: "expired" }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response("not found", { status: 404 });
      }),
    );
    const user = userEvent.setup();
    const onClose = vi.fn();
    show(<ContractStatusModal contract={PREDECESSOR} open onClose={onClose} />);

    await user.click(await screen.findByRole("combobox"));
    await user.click(screen.getByRole("option", { name: "Expired" }));
    await user.click(screen.getByRole("button", { name: "Change status" }));

    await waitFor(() => expect(posted).not.toBeNull());
    const sent = posted as unknown as {
      body: { status?: string };
      ifMatch: string | null;
      path: string;
    };
    expect(sent.path).toBe("/v1/contracts/c-1/status");
    expect(sent.ifMatch).toBe("3");
    expect(sent.body.status).toBe("expired");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });
});

describe("ContractCancelModal", () => {
  it("posts notice and effective dates with the row's version as If-Match", async () => {
    let posted: { body: unknown; ifMatch: string | null; path: string } | null =
      null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(input, init);
        const url = new URL(request.url);
        if (
          request.method === "POST" &&
          url.pathname.endsWith("/cancellation")
        ) {
          posted = {
            body: await request.json(),
            ifMatch: request.headers.get("if-match"),
            path: url.pathname,
          };
          return new Response(JSON.stringify(PREDECESSOR), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response("not found", { status: 404 });
      }),
    );
    const user = userEvent.setup();
    const onClose = vi.fn();
    show(<ContractCancelModal contract={PREDECESSOR} open onClose={onClose} />);

    const notice = await screen.findByLabelText(/^Notice given/);
    await user.type(notice, "2026-06-01");
    const effective = screen.getByLabelText(/^Takes effect/);
    await user.type(effective, "2026-09-01");
    await user.click(
      screen.getByRole("button", { name: "Record cancellation" }),
    );

    await waitFor(() => expect(posted).not.toBeNull());
    const sent = posted as unknown as {
      body: {
        cancellation_notice_on?: string;
        cancellation_effective_on?: string;
      };
      ifMatch: string | null;
      path: string;
    };
    expect(sent.path).toBe("/v1/contracts/c-1/cancellation");
    expect(sent.ifMatch).toBe("3");
    expect(sent.body.cancellation_notice_on).toBe("2026-06-01");
    expect(sent.body.cancellation_effective_on).toBe("2026-09-01");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });
});
