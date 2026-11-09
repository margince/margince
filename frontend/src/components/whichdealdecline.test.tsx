// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { jsonResponse } from "../testing/company.fixtures";
import { WhichDealDecline } from "./whichdealdecline";

type Activity = components["schemas"]["Activity"];

const QUESTION: Activity = {
  occurred_precision: "instant",
  offers_email: false,
  id: "task-1",
  kind: "task",
  writable: true,
  subject: "Which deal is this about?",
  occurred_at: "2026-06-01T09:00:00Z",
  created_at: "2026-06-01T09:00:00Z",
  updated_at: "2026-06-01T09:00:00Z",
  captured_by: "system:capture-which-deal",
  source: "which_deal",
  source_system: "which_deal",
  source_activity_id: "mail-1",
  is_done: false,
  version: 1,
  content_state: "available",
  audience: "workspace",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(task: Activity) {
  const declines: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (path.endsWith("/deal-decline")) {
        declines.push({ path, body: await request.json() });
        return jsonResponse({
          declined_deal_ids: ["deal-1", "deal-2"],
          unlinked: false,
          project_unfiled: false,
          project_kept: false,
          questions_closed: 1,
        });
      }
      throw new Error(`Unexpected request: ${request.method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["activity", "task-1"], task);
  client.setQueryData(["tasks"], {});
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WhichDealDecline task={task} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { declines, client };
}

describe("none of these deals", () => {
  it("answers the question for the email it is about, and reads the task lists again", async () => {
    const user = userEvent.setup();
    const { declines, client } = show(QUESTION);
    await user.click(
      screen.getByRole("button", { name: en["tasks.whichDeal.none"] }),
    );
    expect(
      await screen.findByText(en["tasks.whichDeal.noneDone"]),
    ).toBeTruthy();
    expect(declines).toEqual([
      { path: "/activities/mail-1/deal-decline", body: {} },
    ]);
    expect(client.getQueryState(["activity", "task-1"])?.isInvalidated).toBe(
      true,
    );
    expect(client.getQueryState(["tasks"])?.isInvalidated).toBe(true);
  });

  it("is not offered on any other task", () => {
    show({ ...QUESTION, source_system: "commitment" });
    expect(
      screen.queryByRole("button", { name: en["tasks.whichDeal.none"] }),
    ).toBeNull();
  });
});
