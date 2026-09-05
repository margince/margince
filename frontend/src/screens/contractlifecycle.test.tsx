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
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// A predecessor's own organization has exactly one deal on record for these
// tests — enough to prove the picker lists it and sends its id, without a
// second candidate to disambiguate against.
const ORG_DEAL = { id: "d-1", name: "Renewal — 2025 term" };

function stubRenewalFetch(onRenewal: (request: Request) => Promise<Response>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(input, init);
      const url = new URL(request.url);
      if (request.method === "GET" && url.pathname === "/v1/deals") {
        return new Response(
          JSON.stringify({ data: [ORG_DEAL], page: { has_more: false } }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (request.method === "POST" && url.pathname.endsWith("/renewal")) {
        return onRenewal(request);
      }
      return new Response("not found", { status: 404 });
    }),
  );
}

describe("ContractRenewModal", () => {
  it("posts the successor's own title, basis and the predecessor's version as If-Match, with no deal picked", async () => {
    let posted: { body: unknown; ifMatch: string | null; path: string } | null =
      null;
    stubRenewalFetch(async (request) => {
      posted = {
        body: await request.json(),
        ifMatch: request.headers.get("if-match"),
        path: new URL(request.url).pathname,
      };
      return new Response(
        JSON.stringify({ ...PREDECESSOR, id: "c-2", version: 1 }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      );
    });
    const user = userEvent.setup();
    const onClose = vi.fn();
    show(<ContractRenewModal contract={PREDECESSOR} open onClose={onClose} />);

    // Prefilled from the predecessor, both editable.
    const title = await screen.findByLabelText(/^Title/);
    expect(title).toHaveValue("Framework agreement 2024");

    await user.click(screen.getByRole("button", { name: "Renew" }));

    await waitFor(() => expect(posted).not.toBeNull());
    const sent = posted as unknown as {
      body: Record<string, unknown>;
      ifMatch: string | null;
      path: string;
    };
    expect(sent.path).toBe("/v1/contracts/c-1/renewal");
    expect(sent.ifMatch).toBe("3");
    // The FULL body, not two fields of it: RenewContractRequest inherits
    // nothing from the predecessor but the counterparty (which the server
    // derives from the path, not the body) — a regression that leaked
    // value_minor, currency, or an unpicked deal_id would pass a check that
    // only asserted title and value_basis.
    expect(sent.body).toEqual({
      title: "Framework agreement 2024",
      value_basis: "annualized_12m",
      auto_renew: false,
    });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  // margince#3286 (re-measured): a renewal made through the API could always
  // name the deal that won it; the screen path could not, so a renewal
  // recorded here left deal_id null even where the API allowed it.
  it("sends the picked deal's id", async () => {
    let posted: { body: Record<string, unknown> } | null = null;
    stubRenewalFetch(async (request) => {
      posted = { body: await request.json() };
      return new Response(
        JSON.stringify({ ...PREDECESSOR, id: "c-2", version: 1 }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      );
    });
    const user = userEvent.setup();
    const onClose = vi.fn();
    show(<ContractRenewModal contract={PREDECESSOR} open onClose={onClose} />);

    await user.click(await screen.findByRole("combobox", { name: "Deal" }));
    await user.click(
      await screen.findByRole("option", { name: ORG_DEAL.name }),
    );
    await user.click(screen.getByRole("button", { name: "Renew" }));

    await waitFor(() => expect(posted).not.toBeNull());
    expect(
      (posted as unknown as { body: Record<string, unknown> }).body,
    ).toEqual({
      title: "Framework agreement 2024",
      value_basis: "annualized_12m",
      auto_renew: false,
      deal_id: ORG_DEAL.id,
    });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("shows the server's refusal inline rather than closing silently", async () => {
    stubRenewalFetch(
      async () =>
        new Response(
          JSON.stringify({
            code: "validation_error",
            detail: "a term cannot end before it starts",
          }),
          { status: 422, headers: { "Content-Type": "application/json" } },
        ),
    );
    const user = userEvent.setup();
    const onClose = vi.fn();
    show(<ContractRenewModal contract={PREDECESSOR} open onClose={onClose} />);

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

  it("refuses to submit the status the row already carries", async () => {
    // recordAssignment (patch.go) records a SET regardless of whether the new
    // value equals the old one, so a same-status POST still bumps the row's
    // version and writes an audit row + a from==to contract.status_changed
    // event — a write nobody asked for and a no-op that isn't free.
    let posted = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(input, init);
        const url = new URL(request.url);
        if (request.method === "POST" && url.pathname.endsWith("/status")) {
          posted = true;
        }
        return new Response("not found", { status: 404 });
      }),
    );
    const user = userEvent.setup();
    show(
      <ContractStatusModal contract={PREDECESSOR} open onClose={() => {}} />,
    );

    // The Select opens seeded with the row's own status (active) — untouched.
    await screen.findByRole("combobox");
    await user.click(screen.getByRole("button", { name: "Change status" }));

    expect(posted).toBe(false);
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

  it("refuses an effective date after the term already ends", async () => {
    // contract_cancellation_within_term (contractCheckError,
    // contract_lifecycle.go): "a cancellation cannot take effect after the
    // term already ends." Held client-side too, so the control does not
    // enable a submit the server is certain to refuse.
    let posted = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        posted = true;
        return new Response("not found", { status: 404 });
      }),
    );
    const user = userEvent.setup();
    const dated = { ...PREDECESSOR, ends_on: "2026-08-01" };
    show(<ContractCancelModal contract={dated} open onClose={() => {}} />);

    await user.type(
      await screen.findByLabelText(/^Notice given/),
      "2026-06-01",
    );
    await user.type(screen.getByLabelText(/^Takes effect/), "2026-09-01");
    await user.click(
      screen.getByRole("button", { name: "Record cancellation" }),
    );

    expect(posted).toBe(false);
  });

  // CodeRabbit (PR #4002): the reseed effect keyed on [open, contract] — the
  // OBJECT reference. react-query hands back a new object on every refetch of
  // the same row even when nothing the reader can see changed, so a
  // background orgContracts refetch while this modal is open (another tab
  // editing the same contract, a window-focus refetch) replaced `contract`
  // and re-ran the effect, discarding whatever the reader had already typed.
  // Keying on contract.id instead means a REFETCH of the same row leaves the
  // draft alone; only a genuinely different row (or a fresh open) reseeds it.
  it("keeps what the reader typed across a background refetch of the same row", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { rerender } = render(
      <QueryClientProvider client={client}>
        <LocaleProvider>
          <ContractCancelModal contract={PREDECESSOR} open onClose={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText(/^Notice given/),
      "2026-06-01",
    );

    // The SAME row, refetched: a new object, same id, a version bump — the
    // exact shape a background orgContracts refetch hands back.
    const refetched: Contract = { ...PREDECESSOR, version: 4 };
    rerender(
      <QueryClientProvider client={client}>
        <LocaleProvider>
          <ContractCancelModal contract={refetched} open onClose={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    expect(screen.getByLabelText(/^Notice given/)).toHaveValue("2026-06-01");
  });
});
