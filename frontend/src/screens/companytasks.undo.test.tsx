// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { CompanyScreen } from "./companies";
import {
  company360,
  companyBackstop,
  emptySection,
  jsonResponse,
} from "./company.fixtures";
import { stubFetch } from "./company.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const openTask = {
  activity_id: "a-1",
  subject: "Follow up on contract renewal",
  due_at: "2026-08-20T00:00:00Z",
  overdue: false,
  assignee_id: null,
  linked_deal_id: null,
  linked_contact_id: null,
  version: 1,
};

describe("ticking a task on a company's Tasks tab", () => {
  it("confirms it and offers an Undo that reopens the task on the version the tick produced", async () => {
    const user = userEvent.setup();
    const writes: { is_done?: boolean; ifMatch: string | null }[] = [];
    stubFetch(
      async (url, method, request) => {
        if (method === "PATCH" && url.endsWith("/activities/a-1")) {
          const body = (await request.json()) as { is_done?: boolean };
          writes.push({ ...body, ifMatch: request.headers.get("If-Match") });
          return jsonResponse({ version: 4 });
        }
        return companyBackstop(url);
      },
      {
        company360: {
          ...company360,
          next_steps: { ...emptySection, data: [openTask] },
        },
      },
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <CompanyScreen id="o-1" />
            <ToastRegion />
          </ToastProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await user.click(await screen.findByRole("button", { name: /^Tasks/ }));
    await user.click(await screen.findByRole("checkbox", { name: "Done" }));

    expect(await screen.findByText("Task completed")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[1].is_done).toBe(false);
    expect(writes[1].ifMatch).toContain("4");
  });
});
