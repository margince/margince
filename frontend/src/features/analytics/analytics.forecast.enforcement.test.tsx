/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { meFixture } from "../../testing/mefixture";
import { EnforcementSwitch } from "./analytics.forecast.enforcement";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// An admin sees what switching on would leave out. The switch sends back the
// preview it showed, so the server can refuse a picture that changed.
it("shows what switching on leaves out and sends the preview back", async () => {
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      if (url.endsWith("/me")) {
        return jsonResponse(meFixture({ roles: ["admin"] }));
      }
      if (request?.method === "PUT") {
        sent.push(await request.json());
        return jsonResponse({ state: "enforcing" });
      }
      return jsonResponse({
        state: "observing",
        preview: { left_out_count: 3, unknown_count: 1, digest: "seen" },
      });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider>
        <EnforcementSwitch />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(
    await screen.findByText(/leaves 3 deals out of that line/i),
  ).toBeTruthy();
  expect(screen.getByText(/1 deal has not been judged yet/i)).toBeTruthy();

  await userEvent.click(await screen.findByRole("switch"));
  await waitFor(() =>
    expect(sent).toEqual([{ enabled: true, preview_digest: "seen" }]),
  );
});
