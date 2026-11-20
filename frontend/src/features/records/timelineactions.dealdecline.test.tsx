// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
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
type DealRelevance = components["schemas"]["ActivityDealRelevance"];

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

const ON_DEAL: Activity = {
  ...MAIL,
  links: [{ entity_type: "deal", entity_id: "deal-1" }],
};

const UNRELATED: Activity = {
  ...MAIL,
  links: [
    {
      entity_type: "deal",
      entity_id: "deal-1",
      deal_relevance: "unrelated",
      relevance_source: "human",
    },
  ],
};

function answerFor(
  body: { deal_relevance: "relevant" | "unrelated" },
  extra: Partial<DealRelevance> = {},
): DealRelevance {
  return {
    deal_id: "deal-1",
    deal_relevance: body.deal_relevance,
    relevance_source: "human",
    changed: true,
    project_unfiled: false,
    project_kept: false,
    project_filed: false,
    ...extra,
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(
  activity: Activity,
  entityType: "deal" | "company",
  extra: Partial<DealRelevance> = {},
) {
  const marks: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (path === "/me")
        return jsonResponse(
          meFixture({ allow: { activity: ["read", "update"] } }),
        );
      if (path === "/activities/mail-1/deal-relevance") {
        const body = await request.json();
        marks.push(body);
        return jsonResponse(answerFor(body, extra));
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
  return { marks, client };
}

async function openMenu(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", {
      name: "More actions for Invoice run",
    }),
  );
}

describe("whether a row is about the deal", () => {
  it("marks a row not about the deal, and the toast's Undo marks it about it again", async () => {
    const user = userEvent.setup();
    const { marks, client } = show(ON_DEAL, "deal");
    await openMenu(user);
    await user.click(
      await screen.findByRole("button", { name: en["dealDecline.action"] }),
    );
    expect(await screen.findByText(en["dealDecline.done"])).toBeTruthy();
    expect(
      client.getQueryState(["activities", "deal", "deal-1"])?.isInvalidated,
    ).toBe(true);
    await user.click(screen.getByRole("button", { name: en["common.undo"] }));
    expect(await screen.findByText(en["dealRelevance.restored"])).toBeTruthy();
    expect(marks).toEqual([
      { deal_id: "deal-1", deal_relevance: "unrelated" },
      { deal_id: "deal-1", deal_relevance: "relevant" },
    ]);
  });

  it("says so when the project the row got from the deal stays", async () => {
    const user = userEvent.setup();
    show(ON_DEAL, "deal", { project_kept: true });
    await openMenu(user);
    await user.click(
      await screen.findByRole("button", { name: en["dealDecline.action"] }),
    );
    expect(
      await screen.findByText(en["dealDecline.doneProjectKept"]),
    ).toBeTruthy();
  });

  it("shows a row marked not about the deal, and offers to take the mark back", async () => {
    const user = userEvent.setup();
    const { marks } = show(UNRELATED, "deal");
    expect(screen.getByText(en["dealRelevance.unrelated"])).toBeTruthy();
    await openMenu(user);
    await user.click(
      await screen.findByRole("button", { name: en["dealRelevance.about"] }),
    );
    expect(await screen.findByText(en["dealRelevance.restored"])).toBeTruthy();
    expect(marks).toEqual([{ deal_id: "deal-1", deal_relevance: "relevant" }]);
  });

  it("shows a row the classifier was unsure about", () => {
    show(
      {
        ...MAIL,
        links: [
          {
            entity_type: "deal",
            entity_id: "deal-1",
            deal_relevance: "uncertain",
            relevance_source: "classifier",
          },
        ],
      },
      "deal",
    );
    expect(screen.getByText(en["dealRelevance.uncertain"])).toBeTruthy();
  });

  it("is not offered off a deal timeline, or on a task", async () => {
    const user = userEvent.setup();
    show(MAIL, "company");
    await openMenu(user);
    await screen.findByRole("button", { name: en["compose.relink"] });
    expect(
      screen.queryByRole("button", { name: en["dealDecline.action"] }),
    ).toBeNull();
    cleanup();
    show({ ...ON_DEAL, kind: "task" }, "deal");
    await openMenu(user);
    await screen.findByRole("button", { name: en["compose.relink"] });
    expect(
      screen.queryByRole("button", { name: en["dealDecline.action"] }),
    ).toBeNull();
  });
});
