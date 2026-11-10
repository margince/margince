// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RowName } from "./worklist.rowname";

const client = new QueryClient();
afterEach(() => {
  cleanup();
  client.clear();
  vi.unstubAllGlobals();
});
it("retains the linked record's name beside a translated task title", async () => {
  client.setQueryData(["me"], {
    ...meFixture(),
    reader_rendering_available: true,
  });
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(
        private callback: (entries: Array<{ isIntersecting: boolean }>) => void,
      ) {}
      observe() {
        this.callback([{ isIntersecting: true }]);
      }
      disconnect() {}
    },
  );
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            activity_id: "task-1",
            source_version: 7,
            part: "subject",
            target_language: "en",
            subject: "Review the contract",
            unchanged: false,
            generated_at: "2026-11-09T00:00:00Z",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    ),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RowName
          title="Den Vertrag prüfen · Example Contact"
          item={{
            id: "task-1",
            title: "Den Vertrag prüfen",
            source: "task",
            category: "tasks",
            level: 2,
            consequence: "task_slips",
            occurred_precision: "instant",
            because: [],
            actions: [],
            system_display: {
              locale: "en",
              source_version: 7,
              generated_parts: ["subject"],
            },
          }}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(
    await screen.findByText("Review the contract · Example Contact"),
  ).toBeTruthy();
  expect(screen.queryByText("Den Vertrag prüfen · Example Contact")).toBeNull();
});
