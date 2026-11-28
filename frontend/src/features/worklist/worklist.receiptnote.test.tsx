// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { activityTimeline } from "../../design-system/activitytimeline";
import { TimelineList } from "../../design-system/timeline";
import { browserZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { vi as vietnamese } from "../../i18n/vi";
import { meFixture } from "../../testing/mefixture";
import { HandledForYouPanel } from "./worklist.handled";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const note: components["schemas"]["Activity"] = {
  id: "01a05500-0000-7000-8000-0000000000a1",
  kind: "note",
  subject: "Risk assessment",
  body: "No reply in three weeks.",
  language: "en",
  version: 4,
  occurred_at: "2026-10-20T10:00:00Z",
  occurred_precision: "instant",
  links: [],
  source: "manual",
  is_done: false,
  offers_email: false,
  created_at: "2026-10-20T10:00:00Z",
  updated_at: "2026-10-20T10:00:00Z",
  captured_by: "agent:passport",
  recorded_by_agent: true,
  created_by: {
    actor_type: "agent",
    actor_id: "agent:passport",
    on_behalf_of_name: "Anna Weber",
  },
};

it("shows the same AI attribution and translation action in Worklist and History", async () => {
  const translated: unknown[] = [];
  const activityReads: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const path = new URL(input.url).pathname;
      if (input.method === "GET" && path.includes("/activities/"))
        activityReads.push(path);
      if (path.endsWith(`/activities/${note.id}/reader-rendering`)) {
        translated.push(await input.json());
        return new Response(
          JSON.stringify({
            activity_id: note.id,
            source_version: 4,
            source_language: "en",
            target_language: "vi",
            subject: "Đánh giá rủi ro",
            body: "Chưa có hồi âm trong ba tuần.",
            unchanged: false,
            model_used: "fixture",
            generated_at: note.occurred_at,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      const body = path.endsWith("/me")
        ? {
            ...meFixture({ allow: { activity: ["read"] } }),
            reader_rendering_available: true,
          }
        : path.endsWith("/worklist/handled")
          ? {
              as_of: "2026-10-20T10:00:00Z",
              truncated: false,
              receipts: [
                {
                  id: "receipt-note",
                  kind: "magic.action.create_activity_note",
                  summary: "Đã lưu ghi chú",
                  occurred_at: note.occurred_at,
                  subject: {
                    type: "activity",
                    id: note.id,
                    label: note.subject,
                  },
                },
              ],
            }
          : path.endsWith(`/activities/${note.id}`)
            ? note
            : undefined;
      if (!body) throw new Error(`Unexpected request ${path}`);
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="vi">
        <section aria-label="Worklist">
          <HandledForYouPanel />
        </section>
        <section aria-label="History">
          <TimelineList
            entries={activityTimeline([note])}
            zone={browserZone()}
          />
        </section>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const chip = vietnamese["trust.agentUnnamedFor"].replace(
    "{name}",
    "Anna Weber",
  );
  for (const region of ["Worklist", "History"]) {
    const row = within(screen.getByRole("region", { name: region }));
    expect(await row.findByText(chip)).toBeTruthy();
    expect(
      row.getByRole("button", { name: vietnamese["readerRendering.show"] }),
    ).toBeTruthy();
  }
  const worklist = within(screen.getByRole("region", { name: "Worklist" }));
  await userEvent.click(
    worklist.getByRole("button", { name: vietnamese["readerRendering.show"] }),
  );
  expect(await worklist.findByText("Đánh giá rủi ro")).toBeTruthy();
  expect(worklist.getByText(chip)).toBeTruthy();
  expect(worklist.getByText(note.subject ?? "")).toBeTruthy();
  expect(activityReads).toEqual([`/v1/activities/${note.id}`]);
  expect(translated).toEqual([{ locale: "vi", expected_source_version: 4 }]);
});
