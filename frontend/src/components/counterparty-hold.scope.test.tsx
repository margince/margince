// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import {
  CounterpartyHoldRow,
  type HoldScope,
  holdCoveringAddress,
  holdCoveringDomain,
} from "./counterparty-hold";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderRow(scope: HoldScope, holds: unknown[] = []): unknown[] {
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    "GET /capture/counterparty-holds": () => jsonResponse({ data: holds }),
    "POST /capture/counterparty-holds": (body) => {
      posted.push(body);
      return jsonResponse({}, 201);
    },
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CounterpartyHoldRow scope={scope} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return posted;
}

async function confirm(pressed: RegExp): Promise<void> {
  fireEvent.click(await screen.findByRole("button", { name: pressed }));
  fireEvent.click(await screen.findByRole("button", { name: "Keep private" }));
}

describe("an address scope", () => {
  it("offers both verbs and posts the kind that was pressed", async () => {
    const posted = renderRow({ address: "Office@Firm.example" });
    await confirm(/with this address/);
    await vi.waitFor(() =>
      expect(posted).toEqual([
        { kind: "address", value: "office@firm.example" },
      ]),
    );
    expect(
      screen.getByRole("button", { name: /everyone at firm\.example/ }),
    ).toBeInTheDocument();
  });

  it("posts the domain when the domain verb is pressed", async () => {
    const posted = renderRow({ address: "office@firm.example" });
    await confirm(/everyone at firm\.example/);
    await vi.waitFor(() =>
      expect(posted).toEqual([{ kind: "domain", value: "firm.example" }]),
    );
  });
});

describe("a domain scope", () => {
  it("offers only the domain verb, in company wording, and posts a domain", async () => {
    const posted = renderRow({ domain: "Firm.example" });
    expect(await screen.findByText(/with this company/)).toBeInTheDocument();
    expect(screen.queryByText(/with this address/)).toBeNull();
    await confirm(/everyone at firm\.example/);
    await vi.waitFor(() =>
      expect(posted).toEqual([{ kind: "domain", value: "firm.example" }]),
    );
  });

  it("stands down for an empty value", () => {
    renderRow({ domain: "  " });
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("coverage", () => {
  const holds = [
    { id: "a", kind: "address", value: "office@firm.example", created_at: "" },
    { id: "d", kind: "domain", value: "parent.example", created_at: "" },
  ] as const;

  it("covers a domain by itself or a parent domain hold", () => {
    expect(holdCoveringDomain(holds, "sub.parent.example")?.id).toBe("d");
    expect(holdCoveringDomain(holds, "parent.example")?.id).toBe("d");
  });

  it("never covers a domain by an address hold", () => {
    expect(holdCoveringDomain(holds, "firm.example")).toBeUndefined();
  });

  it("covers an address by an address hold or a domain hold", () => {
    expect(holdCoveringAddress(holds, "Office@firm.example")?.id).toBe("a");
    expect(holdCoveringAddress(holds, "x@sub.parent.example")?.id).toBe("d");
    expect(holdCoveringAddress(holds, "other@firm.example")).toBeUndefined();
  });

  it("picks the narrowest of overlapping domain holds", () => {
    const nested = [
      { id: "p", kind: "domain", value: "example.com", created_at: "" },
      { id: "s", kind: "domain", value: "sub.example.com", created_at: "" },
    ] as const;
    expect(holdCoveringDomain(nested, "sub.example.com")?.id).toBe("s");
    expect(holdCoveringAddress(nested, "a@sub.example.com")?.id).toBe("s");
  });

  it("compares internationalised domains in their stored xn-- form", () => {
    const idn = [
      {
        id: "i",
        kind: "domain",
        value: "xn--mnich-kva.example",
        created_at: "",
      },
    ] as const;
    expect(holdCoveringAddress(idn, "p@münich.example")?.id).toBe("i");
    expect(holdCoveringDomain(idn, "münich.example.")?.id).toBe("i");
  });
});
