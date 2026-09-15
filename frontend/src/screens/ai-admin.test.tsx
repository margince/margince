// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiBudgetCard, AiFeaturesCard, AiFeatureTable } from "./ai-admin";
import { allowance, feature, status } from "./ai-admin.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function mount(
  allow: GrantSpec = {
    ai_budget: ["read", "update"],
    ai_diagnostics: ["read"],
    ai_routing: ["read"],
  },
  conflict = false,
  snapshot = status,
) {
  const writes: unknown[] = [];
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input, init);
    const path = new URL(req.url).pathname;
    let body: unknown = snapshot.budget;
    if (path.endsWith("/me")) body = meFixture({ allow });
    else if (path.endsWith("/ai/status")) body = snapshot;
    else if (req.method === "POST")
      body = {
        current: allowance,
        proposed: {
          ...allowance,
          monthly_tokens: 40000000,
          remaining_tokens: 17546514,
          band: "normal",
        },
        features: status.features,
        deferred_work: status.deferred_work,
      };
    else if (req.method === "PUT") {
      writes.push(await req.json());
      if (conflict)
        return new Response(
          JSON.stringify({
            type: "about:blank",
            title: "Settings changed",
            status: 409,
            code: "version_skew",
          }),
          { status: 409, headers: { "Content-Type": "application/json" } },
        );
    }
    return new Response(JSON.stringify(body), {
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", mock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AiBudgetCard />
        <AiFeaturesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { writes, mock };
}
it("explains pooled tokens, UTC reset and unchanged model selection", async () => {
  mount();
  expect(
    await screen.findByText(/22,453,486 of 24,000,000 tokens/),
  ).toBeTruthy();
  expect(
    screen.getByText(/not an individual quota or a dollar spending cap/),
  ).toBeTruthy();
  expect(await screen.findByText("Same model selection")).toBeTruthy();
  expect(screen.getByText(/UTC/)).toBeTruthy();
  expect(screen.queryByText(/own hardware/)).toBeNull();
});
it("previews both fields before writing the allowance with its revision", async () => {
  const user = userEvent.setup({ delay: null });
  const { writes } = mount();
  await user.click(
    await screen.findByRole("button", { name: "Edit allowance" }),
  );
  const field = screen.getByLabelText("Tokens per full user per month");
  await user.clear(field);
  await user.type(field, "20000000");
  expect(screen.getByRole("button", { name: "Save allowance" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Preview effects" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Save allowance" }),
    ).not.toBeDisabled(),
  );
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "Save allowance" }));
  await screen.findByText("Allowance saved");
  expect(writes).toEqual([
    {
      config: { tokens_per_full_user: 20000000, company_monthly_tokens: null },
      expected_revision: "budget-v1",
    },
  ]);
});
it("keeps a rejected draft visible after a concurrent edit", async () => {
  const user = userEvent.setup({ delay: null });
  mount(undefined, true);
  await user.click(
    await screen.findByRole("button", { name: "Edit allowance" }),
  );
  await user.click(screen.getByRole("button", { name: "Preview effects" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Save allowance" }),
    ).not.toBeDisabled(),
  );
  await user.click(screen.getByRole("button", { name: "Save allowance" }));
  expect(
    await screen.findByText("The change could not be applied"),
  ).toBeTruthy();
  expect(screen.getByLabelText("Tokens per full user per month")).toHaveValue(
    "12000000",
  );
});
it("allows management to read without offering an editor", async () => {
  mount({ ai_budget: ["read"], ai_diagnostics: ["read"] });
  await screen.findByText(/22,453,486 of 24,000,000 tokens/);
  expect(screen.queryByRole("button", { name: "Edit allowance" })).toBeNull();
});

// A reader holding only one of ai_diagnostics:read / ai_budget:read gets the
// withheld panel rather than a section that silently renders nothing:
// `/ai/status` refuses both grant combinations server-side, so there is no
// partial table to show either reader.
it.each([
  { ai_diagnostics: ["read"] } satisfies GrantSpec,
  { ai_budget: ["read"] } satisfies GrantSpec,
])("explains the withheld AI-activity section for %j", async (allow) => {
  mount(allow);
  expect(await screen.findByText("AI by activity")).toBeTruthy();
  expect(
    screen.getByText(
      "Only a reader who holds both AI diagnostics read and AI allowance read can see which features are live right now.",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("table")).toBeNull();
});

it("edits the normal leading tier while the effective tier is demoted", async () => {
  const onEdit = vi.fn();
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable rows={[feature]} onEdit={onEdit} />
    </LocaleProvider>,
  );
  const user = userEvent.setup({ delay: null });
  await user.click(screen.getByText("gemini · example-model"));
  await user.click(screen.getByRole("button", { name: "Edit shared binding" }));
  expect(onEdit).toHaveBeenCalledWith("cheap_cloud");
});
it("explains each routing impact in operational language", () => {
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable
        rows={[
          {
            ...feature,
            task: "blocked",
            display_name: "Blocked activity",
            impact: "budget_blocked",
          },
          {
            ...feature,
            task: "changed",
            display_name: "Changed activity",
            impact: "model_changed",
          },
          {
            ...feature,
            task: "fallback",
            display_name: "Fallback activity",
            impact: "fallback_changed",
          },
          {
            ...feature,
            task: "unconfigured",
            display_name: "Unconfigured activity",
            impact: "unconfigured",
          },
        ]}
      />
    </LocaleProvider>,
  );

  expect(screen.getByText("Waiting on allowance")).toBeTruthy();
  expect(screen.getByText("Different model selected")).toBeTruthy();
  expect(screen.getByText("Fallback chain changed")).toBeTruthy();
  expect(screen.getByText("No model configured")).toBeTruthy();
});
it("withholds model identities from an allowance reader without routing access", async () => {
  mount({ ai_budget: ["read"], ai_diagnostics: ["read"] });
  await screen.findByText(/22,453,486 of 24,000,000 tokens/);
  expect(screen.queryByText("gemini · example-model")).toBeNull();
});
it("reports a failed carrier reading as unavailable", async () => {
  mount();
  const user = userEvent.setup({ delay: null });
  await user.click(
    await screen.findByText("Recorded work waiting on the allowance"),
  );
  expect(screen.getByText(/Account scans: Unavailable/)).toBeTruthy();
});
it("explains the one-user floor when there are no eligible full users", async () => {
  mount(undefined, false, {
    ...status,
    budget: { ...allowance, eligible_full_users: 0, budgeted_full_users: 1 },
  });
  expect(
    await screen.findByText(
      /With no eligible users, the allowance counts one user/,
    ),
  ).toBeTruthy();
});
it("saves a fixed company override without discarding the per-user value", async () => {
  const user = userEvent.setup({ delay: null });
  const { writes } = mount();
  await user.click(
    await screen.findByRole("button", { name: "Edit allowance" }),
  );
  await user.type(
    screen.getByLabelText("Fixed company total (optional)"),
    "35000000",
  );
  await user.click(screen.getByRole("button", { name: "Preview effects" }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "Save allowance" }),
    ).not.toBeDisabled(),
  );
  await user.click(screen.getByRole("button", { name: "Save allowance" }));
  await screen.findByText("Allowance saved");
  expect(writes).toEqual([
    {
      config: {
        tokens_per_full_user: 12000000,
        company_monthly_tokens: 35000000,
      },
      expected_revision: "budget-v1",
    },
  ]);
});
