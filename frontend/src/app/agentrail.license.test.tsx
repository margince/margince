/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { AgentRail } from "./agentrail";
import { meFixture } from "./mefixture";

// The licence pill in the panel's runtime strip names a fault only the seats
// page can repair, so the pill is the way there: a label that told the reader
// to go somewhere it would not take them is a dead end dressed as a warning.
//
// They live apart from agentrail.test.tsx because that file is at the size a
// test file may grow to.

type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// Every read the section makes, answered with the least the panel needs, and
// the licence answered by the case itself. The seat holds `license:read`,
// because a seat without it gets no pill at all.
function stubApi(state: LicenseEntitlement["state"]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      if (pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: { license: ["read"] } }));
      }
      if (pathname.endsWith("/installation/license")) {
        const entitlement: LicenseEntitlement = {
          state,
          seats_used: 1,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        };
        return jsonResponse(entitlement);
      }
      if (pathname.endsWith("/me/ai-activity")) {
        return jsonResponse({ running: [], recent: [] });
      }
      if (pathname.endsWith("/connectors")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({
        data: [],
        page: { has_more: false, next_cursor: null },
      });
    }),
  );
}

async function openPanel(state: LicenseEntitlement["state"]) {
  stubApi(state);
  const user = userEvent.setup();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AgentRail route={{ screen: "deals" }} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const trigger = container.querySelector(".arhit");
  if (!trigger) throw new Error("no .arhit trigger in the rendered tree");
  await user.click(trigger);
  const opened = document.querySelector(".arpanel");
  if (!opened) throw new Error("no .arpanel on the document after the click");
  return opened;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the licence pill leads to the seats settings page", () => {
  it("links a missing licence to the seats page", async () => {
    const opened = await openPanel("absent");
    await waitFor(() => {
      const pill = opened.querySelector("a.arwarning");
      expect(pill?.textContent).toBe("No license");
      expect(pill?.getAttribute("href")).toBe("#/settings/seats");
    });
  });

  // The refused case is the same geometry with different words in it, and the
  // repair is on the same page.
  it("links a refused licence to the seats page", async () => {
    const opened = await openPanel("rejected");
    await waitFor(() => {
      const pill = opened.querySelector("a.arwarning");
      expect(pill?.textContent).toBe("License refused");
      expect(pill?.getAttribute("href")).toBe("#/settings/seats");
    });
  });
});
