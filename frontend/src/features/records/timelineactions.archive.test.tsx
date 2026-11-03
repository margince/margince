// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { jsonResponse } from "../../testing/company.fixtures";
import { meFixture } from "../../testing/mefixture";
import { TimelineActions } from "./timelineactions";

type Activity = components["schemas"]["Activity"];
const NOTE: Activity = {
  occurred_precision: "instant",
  offers_email: false,
  id: "note-1",
  kind: "note",
  writable: true,
  archivable: true,
  subject: "Spoke on LinkedIn",
  occurred_at: "2026-06-01T09:00:00Z",
  created_at: "2026-06-01T09:00:00Z",
  updated_at: "2026-06-01T09:00:00Z",
  captured_by: "human:u1",
  source: "manual",
  is_done: false,
  version: 3,
  content_state: "available",
  audience: "workspace",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(activity: Activity) {
  const archives: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (path === "/me")
        return jsonResponse(
          meFixture({ allow: { activity: ["read", "update"] } }),
        );
      if (path === "/activities/note-1" && request.method === "DELETE") {
        archives.push(request.clone());
        return jsonResponse(activity);
      }
      throw new Error(`Unexpected request: ${request.method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["company360", "company-1"], {});
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <TimelineActions
            activity={activity}
            entityType="company"
            entityId="company-1"
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { archives, client };
}

async function openMenu() {
  await userEvent.click(
    await screen.findByRole("button", {
      name: "More actions for Spoke on LinkedIn",
    }),
  );
}

describe("archiving from the History", () => {
  it("archives a row the server says this reader may, after a confirm", async () => {
    const { archives, client } = show(NOTE);
    await openMenu();
    await userEvent.click(
      await screen.findByRole("button", { name: en["activity.archive"] }),
    );
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["activity.archive"],
      }),
    );
    expect(await screen.findByText(en["activity.archiveDone"])).toBeTruthy();
    expect(archives).toHaveLength(1);
    expect(archives[0]?.headers.get("If-Match")).toContain("3");
    expect(
      client.getQueryState(["company360", "company-1"])?.isInvalidated,
    ).toBe(true);
  });

  it("refuses with the reason on a row the server says is not this reader's to archive", async () => {
    const { archives } = show({ ...NOTE, archivable: false });
    await openMenu();
    const archive = await screen.findByRole("button", {
      name: en["activity.archive"],
    });
    expect(archive.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(en["activity.archiveRefused"])).toBeTruthy();
    expect(archives).toHaveLength(0);
  });
});
