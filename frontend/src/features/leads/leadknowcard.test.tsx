/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { LeadKnowCard } from "./leadcards";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("lists stored factors with their raw values when the catalog endpoint is missing", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) =>
      String(input instanceof Request ? input.url : input).endsWith(
        "/lead-signal-catalog",
      )
        ? Response.json({ title: "Not Found", status: 404 }, { status: 404 })
        : Response.json({
            data: [
              {
                factor: "fleet_size",
                band: "huge",
                points: 5,
                signal_kind: "assumption",
                confidence: null,
                reason: "Heard at the fair",
                set_by: "11111111-1111-4111-8111-111111111111",
                set_at: "2026-10-01T10:00:00Z",
                superseded_at: null,
                superseded_by: null,
              },
            ],
          }),
    ),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <LeadKnowCard id="l-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  expect(await screen.findByText("fleet_size")).toBeTruthy();
  expect(screen.getByText(/huge/)).toBeTruthy();
});
