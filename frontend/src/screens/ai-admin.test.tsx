// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiBudgetCard } from "./ai-admin";
import { allowance, feature, status } from "./ai-admin.testkit";
import { AiFeatureTable } from "./ai-feature-table";

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
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { writes, mock };
}
it("explains pooled tokens, UTC reset and unchanged model selection", async () => {
  mount();
  expect(
    await screen.findByText(/22\.5M of 24M tokens used · 94%/),
  ).toBeTruthy();
  expect(screen.getByText(/1\.5M tokens remaining/)).toBeTruthy();
  expect(
    screen.getByText("Active full users: 2 × 12M tokens per user per month."),
  ).toBeTruthy();
  expect(
    screen.getByText(/Not an individual quota or a dollar spending cap/),
  ).toBeTruthy();
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
// A carrier the server could not count is named as such, never as zero: a
// zero would read as "nothing waiting" on a queue nobody looked at.
it("names a carrier the preview could not count as unavailable", async () => {
  const user = userEvent.setup({ delay: null });
  mount();
  await user.click(
    await screen.findByRole("button", { name: "Edit allowance" }),
  );
  await user.click(screen.getByRole("button", { name: "Preview effects" }));
  const waiting = await screen.findByText(
    "Recorded work waiting on the allowance",
  );
  const list = waiting.closest("details");
  if (!(list instanceof HTMLElement)) {
    throw new Error("the deferred work is not a disclosure");
  }
  expect(within(list).getByText(/Company scans:\s*Unavailable/)).toBeTruthy();
  expect(within(list).getByText(/Website reads:\s*3/)).toBeTruthy();
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
  expect(await screen.findByText("Change not applied")).toBeTruthy();
  expect(screen.getByLabelText("Tokens per full user per month")).toHaveValue(
    "12000000",
  );
});
it("allows management to read without offering an editor", async () => {
  mount({ ai_budget: ["read"], ai_diagnostics: ["read"] });
  await screen.findByText(/22\.5M of 24M tokens used/);
  expect(screen.queryByRole("button", { name: "Edit allowance" })).toBeNull();
});

// A task's tier is fixed by the task contract, and what the tier is bound to
// is edited on the Model tiers card — so the task table offers no edit of its
// own, only the resolved chain and where the task sits in the contract.
it("reads a task's resolved chain with no edit control", async () => {
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable rows={[feature]} />
    </LocaleProvider>,
  );
  const user = userEvent.setup({ delay: null });
  expect(
    screen.getByText(`${feature.task} · ${feature.execution_mode}`),
  ).toBeTruthy();
  await user.click(screen.getByText("example-model"));
  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.queryByText(/edit shared binding/i)).toBeNull();
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
            task: "decision",
            display_name: "Decision activity",
            impact: "decision_changed",
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
  expect(screen.getByText("Decision model changed")).toBeTruthy();
  expect(screen.getByText("Fallback chain changed")).toBeTruthy();
  expect(screen.getByText("No model configured")).toBeTruthy();
});
it("withholds model identities from an allowance reader without routing access", async () => {
  mount({ ai_budget: ["read"], ai_diagnostics: ["read"] });
  await screen.findByText(/22\.5M of 24M tokens used/);
  expect(screen.queryByText("example-model")).toBeNull();
});
it("explains the one-user floor when there are no eligible full users", async () => {
  mount(undefined, false, {
    ...status,
    budget: { ...allowance, eligible_full_users: 0, budgeted_full_users: 1 },
  });
  expect(
    await screen.findByText(
      /With no eligible users, the allowance counts 1 user/,
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
it("puts decision-first rows on top, then rows that declare a skip, then the rest", () => {
  const row = (task: string, extra: Partial<typeof feature> = {}) => ({
    ...feature,
    task,
    display_name: `Activity ${task}`,
    ...extra,
  });
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable
        rows={[
          row("a_plain"),
          row("b_skipped", { decision_skip_reason: "unbound" }),
          row("c_first", {
            decision_first: true,
            decision_candidate: {
              tier: "decide",
              provider: "jev_compatible",
              model: "jev-classify",
              processing: "cloud_provider",
            },
          }),
          row("d_plain"),
          row("e_first", { decision_first: true }),
        ]}
      />
    </LocaleProvider>,
  );

  const order = screen
    .getAllByRole("row")
    .slice(1)
    .map((tr) => within(tr).getByText(/^Activity /).textContent);
  expect(order).toEqual([
    "Activity c_first",
    "Activity e_first",
    "Activity b_skipped",
    "Activity a_plain",
    "Activity d_plain",
  ]);
  expect(screen.getAllByText("Decision model first")).toHaveLength(2);
});

it("shows a badge only for a departure, and no disclosure for a single candidate", () => {
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable
        rows={[
          feature,
          {
            ...feature,
            task: "embed",
            display_name: "Embed",
            budget_exempt: true,
          },
        ]}
      />
    </LocaleProvider>,
  );
  expect(screen.getByText("Continues beyond allowance")).toBeTruthy();
  expect(screen.queryByText("Decision model first")).toBeNull();
  expect(screen.queryAllByRole("group")).toHaveLength(0);
});

it("names only the lead of a multi-candidate row, not the rungs behind it", () => {
  const [lead] = feature.effective_candidates;
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable
        rows={[
          {
            ...feature,
            effective_candidates: [
              lead,
              { ...lead, tier: "cheap_cloud", model: "fallback-model" },
            ],
          },
        ]}
      />
    </LocaleProvider>,
  );
  expect(screen.getByText("example-model")).toBeTruthy();
  expect(screen.queryByText(/fallback-model/)).toBeNull();
});

it("the task table says decision model first, and why another feature skips it", async () => {
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable
        rows={[
          {
            ...feature,
            task: "capture_classify",
            display_name: "Classify correspondence",
            decision_first: true,
            decision_candidate: {
              tier: "decide",
              provider: "jev_compatible",
              model: "jev-classify",
              processing: "cloud_provider",
            },
          },
          {
            ...feature,
            task: "deep_read_triage",
            display_name: "Triage a site",
            decision_skip_reason: "local_only",
          },
        ]}
      />
    </LocaleProvider>,
  );

  // The lane leads, where it processes, and the ladder that answers after it.
  const decisionRow = (await screen.findByText("jev-classify")).closest("td");
  expect(decisionRow?.textContent).toBe(
    "jev_compatiblejev-classify↓thengeminiexample-model",
  );
  // A feature the lane does not serve keeps its ladder, with the reason beside it.
  expect(screen.getAllByText("example-model")).toHaveLength(2);
  expect(
    screen.getByText(
      "Decision model not used: this activity takes only a local decision provider.",
    ),
  ).toBeInTheDocument();
});
