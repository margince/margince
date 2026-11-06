// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { CompanyZoneProvider } from "../../lib/companyzone";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import { day, row, stub } from "../../testing/worklist.testkit";
import { WorklistScreen } from "./worklist";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The server buckets a task's due day on the company's clock (decision B1),
// so the row names that day, even for a reader far from the company. A task
// due by the end of 10 September in New York is stored as 03:59:59Z on the
// 11th, which a reader in Ho Chi Minh City would otherwise read as the 11th.
it("names a task's due day on the company's clock, not the reader's", async () => {
  stub(
    day({
      queue: [
        row({
          id: "t1",
          source: "task",
          category: "tasks",
          title: "Send the signed order",
          due_at: "2026-09-11T03:59:59Z",
          due_has_time: false,
        }),
      ],
      summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyZoneProvider zone="America/New_York">
          <ViewerZoneProvider zone="Asia/Ho_Chi_Minh">
            <WorklistScreen />
          </ViewerZoneProvider>
        </CompanyZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const title = await screen.findByText("Send the signed order");
  const rowText = title.closest(
    "li, article, [role='row'], .worklist-row",
  )?.textContent;
  expect(rowText).toContain("10/09/2026");
  expect(rowText).not.toContain("11/09/2026");
});
