/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { RequestsPanel } from "./settings.privacy";

// The Requests panel: three lists one officer works, one at a time. Each has
// its own verb in the head and its count on the switch.

const OFFICER: GrantSpec = {
  privacy_request: ["read", "update"],
  retention_policy: ["read", "update"],
  contact: ["update"],
};

const page = (data: unknown[], hasMore = false) => ({
  data,
  page: { next_cursor: hasMore ? "next" : null, has_more: hasMore },
});

const DSR = {
  id: "d1",
  kind: "access",
  subject_ref: "partner@acme.test",
  status: "open",
  due_at: "2099-10-01T00:00:00Z",
  due_overridden: false,
  dispute_open: false,
  created_at: "2026-08-01T00:00:00Z",
};

const CORRECTION = {
  id: "c1",
  contact_id: "c-1",
  kind: "correction",
  field: "full_name",
  proposed_value: "Schmidt",
  current_value: "Schmitt",
  contact_name: "Anna Schmidt",
  submitted_at: "2026-08-01T09:00:00Z",
};

const HELD = {
  activity_id: "00000000-0000-4000-8000-0000000000b1",
  kind: "email",
  occurred_at: "2025-03-04T09:00:00Z",
  restricted_at: "2026-08-18T07:00:00Z",
  restricted_until: "2032-01-01T00:00:00Z",
  reason: "commercial_correspondence · §257 HGB / §147 AO",
  deals: [],
  redacted_fields: [],
};

function backend({
  dsrs = [DSR],
  corrections = [CORRECTION],
  correctionsHaveMore = false,
}: {
  dsrs?: unknown[];
  corrections?: unknown[];
  correctionsHaveMore?: boolean;
} = {}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url, "https://test.local");
      const path = url.pathname.replace(/^\/v1/, "");
      const body = (value: unknown) =>
        new Response(JSON.stringify(value), {
          headers: { "Content-Type": "application/json" },
        });
      if (path === "/me") return body(meFixture({ allow: OFFICER }));
      if (path === "/data-subject-requests") {
        return body(page(url.searchParams.get("status") ? [] : dsrs));
      }
      if (path === "/confirm-submissions") {
        return body(page(corrections, correctionsHaveMore));
      }
      if (path === "/retention/restrictions") return body(page([HELD]));
      return body(page([]));
    }),
  );
}

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RequestsPanel />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return within(screen.getByRole("region", { name: "Requests" }));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("RequestsPanel", () => {
  it("shows one list at a time, and keeps focus on the switch that chose it", async () => {
    backend();
    const user = userEvent.setup();
    const panel = renderPanel();
    expect(await panel.findByText("partner@acme.test")).toBeVisible();

    const corrections = panel.getByRole("button", { name: /^Corrections/ });
    await user.click(corrections);
    expect(await panel.findByText("Anna Schmidt")).toBeVisible();
    expect(panel.queryByText("partner@acme.test")).not.toBeInTheDocument();
    expect(corrections).toHaveFocus();
    expect(corrections).toHaveAttribute("aria-pressed", "true");
  });

  it("puts the open list's own verb in the panel head", async () => {
    backend();
    const user = userEvent.setup();
    const panel = renderPanel();

    expect(
      await panel.findByRole("button", { name: en["privacy.newRequest"] }),
    ).toBeVisible();
    await user.click(panel.getByRole("button", { name: /^Corrections/ }));
    expect(
      panel.queryByRole("button", { name: en["privacy.newRequest"] }),
    ).not.toBeInTheDocument();
    expect(
      panel.queryByRole("button", { name: en["restricted.pin.action"] }),
    ).not.toBeInTheDocument();
    await user.click(panel.getByRole("button", { name: /^Held/ }));
    expect(
      panel.getByRole("button", { name: en["restricted.pin.action"] }),
    ).toBeVisible();
  });

  it("counts what waits behind the lists it can count in full", async () => {
    backend();
    const panel = renderPanel();
    expect(
      await panel.findByRole("button", { name: /^Corrections\W+1$/ }),
    ).toBeVisible();
    expect(
      await panel.findByRole("button", { name: /^Held\W+1$/ }),
    ).toBeVisible();
    // The subject queue pages, so it never claims a total.
    expect(panel.getByRole("button", { name: "Privacy" })).toBeVisible();
  });

  it("claims no count for a queue with more pages to load", async () => {
    backend({ correctionsHaveMore: true });
    const panel = renderPanel();
    expect(
      await panel.findByRole("button", { name: /^Held\W+1$/ }),
    ).toBeVisible();
    expect(
      panel.getByRole("button", { name: en["privacy.laneCorrections"] }),
    ).toBeVisible();
  });

  it("offers the status filter only once the queue holds a request", async () => {
    backend({ dsrs: [] });
    const panel = renderPanel();
    expect(await panel.findByText(en["privacy.requestsEmpty"])).toBeVisible();
    expect(
      panel.queryByRole("group", { name: en["privacy.facetLabel"] }),
    ).not.toBeInTheDocument();
  });

  it("keeps a chosen status cut on screen when it holds nothing", async () => {
    backend();
    const user = userEvent.setup();
    const panel = renderPanel();
    const filter = await panel.findByRole("group", {
      name: en["privacy.facetLabel"],
    });
    await user.click(within(filter).getByRole("button", { name: "Fulfilled" }));
    expect(await panel.findByText(en["common.empty"])).toBeVisible();
    expect(
      panel.getByRole("group", { name: en["privacy.facetLabel"] }),
    ).toBeVisible();
  });

  it("says an empty corrections list in one line under its intro", async () => {
    backend({ corrections: [] });
    const user = userEvent.setup();
    const panel = renderPanel();
    await user.click(
      await panel.findByRole("button", { name: /^Corrections/ }),
    );
    const empty = await panel.findByText(en["privacy.correctionsEmpty"]);
    expect(empty.closest(".panel-body")).toBe(
      panel.getByText(en["privacy.correctionsSub"]).closest(".panel-body"),
    );
  });
});
