/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { type Locale, LocaleProvider, translate } from "../../i18n";
import { approvalHref, LinkedApprovalDrawer } from "./approvaldrawer";

// The drawer a notice or a receipt line opens: the decision the address names.

function approval(status: string) {
  return {
    id: "ap1",
    kind: "close_date_correction",
    status,
    summary: "Confirm the real close date",
    proposed_by: "system:close-date",
    proposed_change: {
      deal_id: "d1",
      expected_close_date: "2026-11-15",
      basis: "Nobody has answered since 5 August.",
    },
    created_at: "2026-08-20T09:00:00Z",
    target_entity_type: "deal",
    target_entity_id: "d1",
  };
}

function openAt(
  status: string,
  kind = "close_date_correction",
  locale: Locale = "en",
  assigneeInactive?: boolean,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            ...approval(status),
            kind,
            assignee_inactive: assigneeInactive,
            ...(kind === "commitment_task"
              ? {
                  summary: "Send the revised scope",
                  proposed_change: {
                    summary: "Send the revised scope",
                    party: "Alex Beck",
                  },
                  target_entity_type: null,
                  target_entity_id: null,
                }
              : {}),
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
    ),
  );
  globalThis.location.hash = approvalHref("ap1");
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <ToastProvider>
          <LinkedApprovalDrawer />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

describe("a link to one decision", () => {
  it("opens that decision, answerable", async () => {
    openAt("pending");
    expect(await screen.findByRole("button", { name: "Approve" })).toBeTruthy();
  });

  it("shows a decision already answered as answered, with nothing to press", async () => {
    openAt("approved");
    expect(await screen.findByText("Confirm the real close date")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reject" })).toBeNull();
  });
});

const commitmentLabels: ReadonlyArray<{ locale: Locale; label: string }> = [
  { locale: "en", label: "Create task" },
  { locale: "de", label: "Aufgabe erstellen" },
  { locale: "vi", label: "Tạo 1 công việc" },
];
it.each(commitmentLabels)(
  "names the commitment effect in $locale",
  async ({ locale, label }) => {
    openAt("pending", "commitment_task", locale);
    expect(await screen.findByRole("button", { name: label })).toBeEnabled();
    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: translate(locale, "trust.edit") }),
    );
    expect(await screen.findByRole("button", { name: label })).toBeEnabled();
  },
);

it("explains an inactive assignment and refuses both task creation controls", async () => {
  const user = userEvent.setup();
  openAt("pending", "commitment_task", "en", true);
  expect(await screen.findByText(/cannot receive tasks/)).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Create task" }),
  ).not.toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: translate("en", "trust.edit") }),
  );
  expect(screen.getByRole("button", { name: "Create task" })).toBeDisabled();
});
