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
type DealDecline = components["schemas"]["ActivityDealDecline"];

const MAIL: Activity = {
  occurred_precision: "instant",
  offers_email: false,
  id: "mail-1",
  kind: "email",
  writable: true,
  archivable: false,
  subject: "Invoice run",
  occurred_at: "2026-06-01T09:00:00Z",
  created_at: "2026-06-01T09:00:00Z",
  updated_at: "2026-06-01T09:00:00Z",
  captured_by: "connector:gmail:u1",
  source: "gmail",
  is_done: false,
  version: 2,
  content_state: "available",
  audience: "workspace",
};

const DECLINED: DealDecline = {
  declined_deal_ids: ["deal-1"],
  unlinked: true,
  project_unfiled: true,
  project_kept: false,
  questions_closed: 0,
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(
  activity: Activity,
  entityType: "deal" | "company",
  answer: DealDecline = DECLINED,
) {
  const declines: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (path === "/me")
        return jsonResponse(
          meFixture({ allow: { activity: ["read", "update"] } }),
        );
      if (path === "/activities/mail-1/deal-decline") {
        declines.push(await request.json());
        return jsonResponse(answer);
      }
      throw new Error(`Unexpected request: ${request.method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["activities", "deal", "deal-1"], {});
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <TimelineActions
            activity={activity}
            entityType={entityType}
            entityId={entityType === "deal" ? "deal-1" : "company-1"}
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { declines, client };
}

async function openMenu(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", {
      name: "More actions for Invoice run",
    }),
  );
}

describe("not about this deal", () => {
  it("takes an email off the deal whose timeline shows it, after a confirm", async () => {
    const user = userEvent.setup();
    const { declines, client } = show(MAIL, "deal");
    await openMenu(user);
    await user.click(
      await screen.findByRole("button", { name: en["dealDecline.action"] }),
    );
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["dealDecline.confirm"],
      }),
    );
    expect(await screen.findByText(en["dealDecline.done"])).toBeTruthy();
    expect(declines).toEqual([{ deal_id: "deal-1" }]);
    expect(
      client.getQueryState(["activities", "deal", "deal-1"])?.isInvalidated,
    ).toBe(true);
  });

  it("says so when the project the email got from the deal stays", async () => {
    const user = userEvent.setup();
    show(MAIL, "deal", {
      ...DECLINED,
      project_unfiled: false,
      project_kept: true,
    });
    await openMenu(user);
    await user.click(
      await screen.findByRole("button", { name: en["dealDecline.action"] }),
    );
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: en["dealDecline.confirm"],
      }),
    );
    expect(
      await screen.findByText(en["dealDecline.doneProjectKept"]),
    ).toBeTruthy();
  });

  it("is not offered off a deal timeline, or on a row that is not an email", async () => {
    const user = userEvent.setup();
    show(MAIL, "company");
    await openMenu(user);
    await screen.findByRole("button", { name: en["compose.relink"] });
    expect(
      screen.queryByRole("button", { name: en["dealDecline.action"] }),
    ).toBeNull();
    cleanup();
    show({ ...MAIL, kind: "note" }, "deal");
    await openMenu(user);
    await screen.findByRole("button", { name: en["compose.relink"] });
    expect(
      screen.queryByRole("button", { name: en["dealDecline.action"] }),
    ).toBeNull();
  });
});
