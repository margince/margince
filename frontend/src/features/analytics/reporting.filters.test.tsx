// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { RecordZoneProvider } from "../../lib/recordzone";
import { ReportingFilters } from "./reporting.filters";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("marks today in the workspace zone the custom period is counted in", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  // 22:00 on 31 March in Pago Pago is already 1 April almost everywhere else.
  vi.setSystemTime(new Date("2026-04-01T09:00:00Z"));
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ data: [], page: { next_cursor: null } })),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <RecordZoneProvider zone="Pacific/Pago_Pago">
          <ReportingFilters
            showScope={false}
            showPipeline={false}
            showTargets={false}
            selection={{
              scope: { kind: "workspace" },
              period: "custom",
              target_basis: "month",
              close_window: "all_open",
              metrics: [],
              blocks: [],
            }}
            onChange={vi.fn()}
          />
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();

  await user.click(screen.getByRole("button", { name: "Choose date: From" }));

  const marked = await screen.findByRole("button", { current: "date" });
  expect(marked).toHaveAccessibleName("Tuesday, 31 March 2026");
});
