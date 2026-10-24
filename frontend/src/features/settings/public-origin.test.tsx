/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { useConnectors } from "../../lib/connectors";
import { PublicOriginCard } from "./public-origin";

type PublicOrigin =
  components["schemas"]["CaptureConnectionListResponse"]["public_origin"];

// GET /connectors carries the origin; a 501 is a deployment with no capture.
function stubConnectors(publicOrigin: PublicOrigin, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path.endsWith("/connectors") && request.method === "GET") {
        const body =
          status === 501
            ? { code: "not_implemented", detail: "capture not wired" }
            : { data: [], public_origin: publicOrigin };
        return new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        });
      }
      throw new Error(`unstubbed: ${request.method} ${path}`);
    }),
  );
}

// Shares the card's cache entry, so absence is only judged once it answered.
function ConnectorsSettled() {
  return useConnectors().isSuccess ? <span>connectors settled</span> : null;
}

function render() {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <PublicOriginCard />
        <ConnectorsSettled />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A deployment configured with a localhost origin sends mail whose unsubscribe
// links open nothing. This card is where somebody can see that before a
// recipient clicks one.
describe("the public address card", () => {
  it("shows the address used in emailed links, and that it answers", async () => {
    stubConnectors({
      origin: "https://crm.example.com",
      reachable: true,
      detail: "http 200",
    });
    render();
    expect(
      await screen.findByRole("heading", { name: "Links in outgoing mail" }),
    ).toBeInTheDocument();
    const row = screen.getByTestId("public-origin");
    expect(within(row).getByText("https://crm.example.com")).toBeTruthy();
    expect(within(row).getByText("Reachable")).toBeTruthy();
  });

  it("says so when the address does not answer", async () => {
    stubConnectors({ origin: "https://crm.example.com", reachable: false });
    render();
    expect(
      within(await screen.findByTestId("public-origin")).getByText(
        "Unreachable",
      ),
    ).toBeTruthy();
  });

  // Null is "not asked yet", which must not read as a failure.
  it("distinguishes an unchecked address from a failing one", async () => {
    stubConnectors({ origin: "https://crm.example.com", reachable: null });
    render();
    expect(
      within(await screen.findByTestId("public-origin")).getByText(
        "Not checked",
      ),
    ).toBeTruthy();
  });

  it("draws nothing when no address is configured", async () => {
    stubConnectors(undefined);
    render();
    await screen.findByText("connectors settled");
    expect(screen.queryByTestId("public-origin")).toBeNull();
    expect(
      screen.queryByRole("heading", { name: "Links in outgoing mail" }),
    ).toBeNull();
  });

  it("draws nothing on a deployment without mail capture", async () => {
    stubConnectors(undefined, 501);
    render();
    await screen.findByText("connectors settled");
    expect(screen.queryByTestId("public-origin")).toBeNull();
  });
});
