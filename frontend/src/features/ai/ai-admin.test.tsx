// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { UnsavedGuard, useHasUnsavedChanges } from "../../components/unsaved";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { allowance, feature, status } from "../../testing/ai-admin.testkit";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { AiBudgetCard } from "./ai-admin";
import { AiFeatureTable } from "./ai-feature-table";
import { openTaskDetails } from "./ai-task-details-testing";
import { TeamRulesCard } from "./ai-tasks";

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
  const server = { budget: snapshot.budget };
  const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input, init);
    const path = new URL(req.url).pathname;
    let body: unknown = server.budget;
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
        <UnsavedGuard address="usage" onKeep={() => undefined}>
          {() => (
            <>
              <AiBudgetCard />
              <UnsavedProbe />
            </>
          )}
        </UnsavedGuard>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { writes, mock, server, client };
}

function UnsavedProbe() {
  return (
    <output data-testid="unsaved">
      {useHasUnsavedChanges() ? "dirty" : "clean"}
    </output>
  );
}

async function openEditor(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  );
  return screen.getByLabelText(englishCopy["aiAdmin.perUser"]);
}
it("explains pooled tokens, UTC reset and unchanged model selection", async () => {
  mount();
  expect(await screen.findByText("22.5M of 24M tokens · 94%")).toBeTruthy();
  expect(
    screen.getByText("Active full users: 2 × 12M tokens per user per month."),
  ).toBeTruthy();
  expect(
    screen.getByText(/Not an individual quota or a dollar spending cap/),
  ).toBeTruthy();
  expect(screen.getByText(/UTC/)).toBeTruthy();
  expect(screen.queryByText(/own hardware/)).toBeNull();
});
it("states the band only once it is off normal, and never the spend in money", async () => {
  mount();
  expect(await screen.findByText(englishCopy["aiAdmin.degraded"])).toBeTruthy();
  expect(document.body.textContent).not.toMatch(/[$€£]\s?\d/);
  cleanup();
  mount(undefined, false, {
    ...status,
    budget: { ...status.budget, band: "normal" },
  });
  await screen.findByText("22.5M of 24M tokens · 94%");
  for (const title of [
    "80% threshold reached: reduced-tier routing is active",
    "Allowance reached: background AI is deferred",
  ]) {
    expect(screen.queryByText(title)).toBeNull();
  }
});
it("previews both fields before writing the allowance with its revision", async () => {
  const user = userEvent.setup({ delay: null });
  const { writes } = mount();
  await user.click(
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  );
  expect(
    screen.getByRole("dialog", { name: englishCopy["aiAdmin.allowance"] }),
  ).toBeTruthy();
  const field = screen.getByLabelText(englishCopy["aiAdmin.perUser"]);
  await user.clear(field);
  await user.type(field, "20000000");
  expect(
    screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
  ).toBeDisabled();
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
    ).not.toBeDisabled(),
  );
  expect(writes).toHaveLength(0);
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
  );
  await screen.findByText(englishCopy["aiAdmin.saved"]);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(writes).toEqual([
    {
      config: { tokens_per_full_user: 20000000, company_monthly_tokens: null },
      expected_revision: "budget-v1",
    },
  ]);
});
it("says at the field what is wrong with an allowance, and why Preview is held", async () => {
  const user = userEvent.setup({ delay: null });
  const { mock } = mount();
  await user.click(
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  );
  const field = screen.getByLabelText(englishCopy["aiAdmin.perUser"]);
  await user.clear(field);
  await user.type(field, "1.5");
  expect(field).toHaveAccessibleDescription(
    /Enter a whole number for Tokens per full user per month\./,
  );
  expect(screen.getByText(englishCopy["aiAdmin.fixToPreview"])).toBeTruthy();
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  const company = screen.getByLabelText(englishCopy["aiAdmin.company"]);
  await user.type(company, "-1");
  expect(company).toHaveAccessibleDescription(
    /Fixed company total must be from 1 to 1,000,000,000,000\./,
  );

  await user.clear(field);
  await user.type(field, " 5 ");
  await user.clear(company);
  expect(screen.queryByText(englishCopy["aiAdmin.fixToPreview"])).toBeNull();
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  await waitFor(() =>
    expect(
      mock.mock.calls.filter(
        ([input, init]) => new Request(input, init).method === "POST",
      ),
    ).toHaveLength(1),
  );
});
it("says one token in the singular", async () => {
  mount(undefined, false, {
    ...status,
    budget: {
      ...status.budget,
      config: { ...status.budget.config, tokens_per_full_user: 1 },
    },
  });
  expect(
    await screen.findByText(
      "Active full users: 2 × 1 token per user per month.",
    ),
  ).toBeTruthy();
});
// A carrier the server could not count is named as such, never as zero: a
// zero would read as "nothing waiting" on a queue nobody looked at.
it("names a carrier the preview could not count as unavailable", async () => {
  const user = userEvent.setup({ delay: null });
  mount();
  await user.click(
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  const waiting = await screen.findByText(englishCopy["aiAdmin.waiting"]);
  const list = waiting.closest("details");
  if (!(list instanceof HTMLElement)) {
    throw new Error("the deferred work is not a disclosure");
  }
  expect(within(list).getByText(/Company scans:\s*Unavailable/)).toBeTruthy();
  expect(within(list).getByText(/Website reads:\s*3/)).toBeTruthy();
});
// The budget count is what a raise would resume; work waiting on the provider
// is its own line, shown only when there is some.
it("shows the provider wait beside the carrier's budget count", async () => {
  const user = userEvent.setup({ delay: null });
  mount();
  await user.click(
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  const waiting = await screen.findByText(englishCopy["aiAdmin.waiting"]);
  const list = waiting.closest("details");
  if (!(list instanceof HTMLElement)) {
    throw new Error("the deferred work is not a disclosure");
  }
  expect(
    within(list).getAllByText(
      "Waiting for the AI provider: 2. It resumes by itself when the provider answers.",
    ),
  ).toHaveLength(1);
});
it("keeps a rejected draft visible after a concurrent edit", async () => {
  const user = userEvent.setup({ delay: null });
  mount(undefined, true);
  await user.click(
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
    ).not.toBeDisabled(),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
  );
  expect(await screen.findByText(englishCopy["aiAdmin.failed"])).toBeTruthy();
  expect(screen.getByLabelText(englishCopy["aiAdmin.perUser"])).toHaveValue(
    "12000000",
  );
});
it("allows management to read without offering an editor", async () => {
  mount({ ai_budget: ["read"], ai_diagnostics: ["read"] });
  await screen.findByText(/22\.5M of 24M tokens/);
  expect(
    screen.queryByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  ).toBeNull();
});

// A task's tier is fixed by the task contract, and what the tier is bound to
// is edited on the Model tiers card — so the task table offers no edit of its
// own, only the resolved chain, whose model id waits behind its provider.
it("reads a task's resolved chain with no edit control", async () => {
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable rows={[feature]} />
    </LocaleProvider>,
  );
  const user = userEvent.setup({ delay: null });
  expect(await openTaskDetails(user, feature.display_name)).toHaveTextContent(
    `${feature.task} · While the user waits`,
  );
  await user.click(screen.getByRole("button", { name: /example-model/ }));
  expect(
    screen.getByRole("region", { name: /example-model/ }),
  ).toHaveTextContent("example-model");
  expect(screen.queryByRole("button", { name: /edit/i })).toBeNull();
  expect(screen.queryByText(/edit shared binding/i)).toBeNull();
});
it("explains each routing impact in operational language", async () => {
  const user = userEvent.setup({ delay: null });
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

  for (const [name, says] of [
    ["Blocked activity", "Waiting on allowance"],
    ["Changed activity", "Different model selected"],
    ["Decision activity", "Decision model changed"],
    ["Fallback activity", "Fallback chain changed"],
    ["Unconfigured activity", "No model configured"],
  ]) {
    expect(await openTaskDetails(user, name)).toHaveTextContent(says);
  }
});
it("withholds model identities from an allowance reader without routing access", async () => {
  mount({ ai_budget: ["read"], ai_diagnostics: ["read"] });
  await screen.findByText(/22\.5M of 24M tokens/);
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
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  );
  await user.type(
    screen.getByLabelText(englishCopy["aiAdmin.company"]),
    "35000000",
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
    ).not.toBeDisabled(),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
  );
  await screen.findByText(englishCopy["aiAdmin.saved"]);
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
it("puts decision-first rows on top, then rows that declare a skip, then the rest", async () => {
  const user = userEvent.setup({ delay: null });
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
    .map((tr) => within(tr).getByText(/^Activity \w+$/).textContent);
  expect(order).toEqual([
    "Activity c_first",
    "Activity e_first",
    "Activity b_skipped",
    "Activity a_plain",
    "Activity d_plain",
  ]);
  for (const [name, first] of [
    ["Activity c_first", true],
    ["Activity a_plain", false],
  ] as const) {
    const details = await openTaskDetails(user, name);
    expect(
      within(details).queryByText(englishCopy["aiTasks.decisionFirst"]) !==
        null,
    ).toBe(first);
  }
});

it("says per task what the records say about the model a tier serves", async () => {
  const user = userEvent.setup({ delay: null });
  const [lead] = feature.effective_candidates;
  if (!lead) throw new Error("the feature fixture carries no candidate");
  const withCertification = (
    task: string,
    certification?: {
      verdict: "certified" | "supported_degraded" | "not_supported";
      current: boolean;
    },
  ) => ({
    ...feature,
    task,
    display_name: `Activity ${task}`,
    effective_candidates: [{ ...lead, model: "m-1", certification }],
  });
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable
        rows={[
          withCertification("a", { verdict: "certified", current: true }),
          withCertification("b", { verdict: "not_supported", current: true }),
          withCertification("c", {
            verdict: "supported_degraded",
            current: false,
          }),
          withCertification("d"),
        ]}
      />
    </LocaleProvider>,
  );
  expect(await openTaskDetails(user, "Activity a")).toHaveTextContent(
    "m-1 is certified for this task.",
  );
  expect(await openTaskDetails(user, "Activity b")).toHaveTextContent(
    "m-1 is not certified for this task. It is still used",
  );
  expect(await openTaskDetails(user, "Activity c")).toHaveTextContent(
    "reduced quality. The record predates the current prompts.",
  );
  expect(await openTaskDetails(user, "Activity d")).toHaveTextContent(
    "m-1 has not been measured for this task with these settings.",
  );
});

it("shows a badge only for a departure, and no disclosure for a single candidate", async () => {
  const user = userEvent.setup({ delay: null });
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
  expect(await openTaskDetails(user, "Embed")).toHaveTextContent(
    englishCopy["aiAdmin.impact.exempt"],
  );
  const quiet = await openTaskDetails(user, feature.display_name);
  expect(quiet).not.toHaveTextContent(englishCopy["aiAdmin.impact.exempt"]);
  expect(quiet).not.toHaveTextContent(englishCopy["aiTasks.decisionFirst"]);
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
  const user = userEvent.setup({ delay: null });
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
            decision_skip_reason: "unbound",
          },
        ]}
      />
    </LocaleProvider>,
  );

  // The row names the model that answers; the decision model in front of it
  // is in the task's details, with its provider.
  const decisionRow = screen
    .getByText(englishCopy["aiTask.name.capture_classify"])
    .closest("tr");
  expect(decisionRow?.querySelector("td:nth-child(3)")?.textContent).toBe(
    "Google Geminiexample-model",
  );
  const first = await openTaskDetails(user, "Message classification");
  expect(first).toHaveTextContent(
    "Decision model firstJev-compatiblejev-classify",
  );
  // A feature the lane does not serve says why, in its own details.
  expect(await openTaskDetails(user, "Triage a site")).toHaveTextContent(
    englishCopy["aiAdmin.decisionSkip.unbound"],
  );
});

it("guards the editor only while a value differs from the saved allowance", async () => {
  const user = userEvent.setup({ delay: null });
  mount();
  const field = await openEditor(user);
  expect(screen.getByTestId("unsaved")).toHaveTextContent("clean");
  await user.clear(field);
  await user.type(field, "20000000");
  expect(screen.getByTestId("unsaved")).toHaveTextContent("dirty");
  await user.clear(field);
  await user.type(field, "12000000");
  expect(screen.getByTestId("unsaved")).toHaveTextContent("clean");
});

it("offers to reload the values when the allowance changed under the editor", async () => {
  const user = userEvent.setup({ delay: null });
  const { server, client } = mount();
  const field = await openEditor(user);
  await user.clear(field);
  await user.type(field, "20000000");
  server.budget = {
    ...allowance,
    config: { tokens_per_full_user: 15000000, company_monthly_tokens: null },
    revision: "budget-v2",
  };
  await client.invalidateQueries({ queryKey: ["ai-budget"] });
  expect(
    await screen.findByText(englishCopy["aiAdmin.stale"]),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  ).toBeDisabled();

  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.reload"] }),
  );
  expect(field).toHaveValue("15000000");
  expect(field).toHaveFocus();
  expect(screen.queryByText(englishCopy["aiAdmin.stale"])).toBeNull();
  expect(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  ).not.toBeDisabled();
});

it("holds no draft when another admin saves under an untouched editor", async () => {
  const user = userEvent.setup({ delay: null });
  const { server, client } = mount();
  await openEditor(user);
  server.budget = {
    ...allowance,
    config: { tokens_per_full_user: 15000000, company_monthly_tokens: null },
    revision: "budget-v2",
  };
  await client.invalidateQueries({ queryKey: ["ai-budget"] });
  expect(
    await screen.findByText(englishCopy["aiAdmin.stale"]),
  ).toBeInTheDocument();
  expect(screen.getByTestId("unsaved")).toHaveTextContent("clean");
});

it("hands focus back to Edit when the editor closes", async () => {
  const user = userEvent.setup({ delay: null });
  mount();
  await openEditor(user);
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(
    await screen.findByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  ).toHaveFocus();

  const field = await openEditor(user);
  await user.clear(field);
  await user.type(field, "20000000");
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.preview"] }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
    ).not.toBeDisabled(),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["aiAdmin.save"] }),
  );
  await screen.findByText(englishCopy["aiAdmin.saved"]);
  expect(
    screen.getByRole("button", { name: englishCopy["aiAdmin.edit"] }),
  ).toHaveFocus();
});

const savedRules = {
  text: "Never quote a discount.",
  updated_at: "2026-11-20T10:00:00Z",
  updated_by: "Ada Admin",
};

function mountTeamRules(allow: GrantSpec) {
  const writes: unknown[] = [];
  const server = { rules: { ...savedRules }, skew: false };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req = new Request(input, init);
      const path = new URL(req.url).pathname;
      let body: unknown = server.rules;
      if (path.endsWith("/me")) body = meFixture({ allow });
      else if (req.method === "PUT") {
        const sent = (await req.json()) as { text: string };
        writes.push(sent);
        if (server.skew)
          return new Response(
            JSON.stringify({
              type: "about:blank",
              title: "Changed",
              status: 409,
              code: "version_skew",
            }),
            { status: 409, headers: { "Content-Type": "application/json" } },
          );
        body = { ...server.rules, text: sent.text };
      }
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SavePageProvider>
          <TeamRulesCard />
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { writes, server, client };
}

it("saves the team rules against the version the editor read", async () => {
  const user = userEvent.setup();
  const { writes } = mountTeamRules({ ai_team_rules: ["read", "update"] });
  const box = await screen.findByRole("textbox", {
    name: englishCopy["teamRules.label"],
  });
  expect(box).toHaveValue("Never quote a discount.");
  expect(screen.getByText(/Last saved by Ada Admin/)).toBeInTheDocument();
  await waitFor(() => expect(box).toBeEnabled());
  await user.type(box, " Write licence.");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        text: "Never quote a discount. Write licence.",
        expected_updated_at: "2026-11-20T10:00:00Z",
      },
    ]),
  );
});

it("shows the team rules read-only to a seat that cannot update them", async () => {
  mountTeamRules({ ai_team_rules: ["read"] });
  expect(
    await screen.findByRole("textbox", {
      name: englishCopy["teamRules.label"],
    }),
  ).toBeDisabled();
});

const theirs = {
  text: "Their text.",
  updated_at: "2026-11-21T09:00:00Z",
  updated_by: "Bo Colleague",
};

it("shows a conflict when the server changes under a draft, saves against the version the draft started from, and Discard shows the fresh text", async () => {
  const user = userEvent.setup();
  const { writes, server, client } = mountTeamRules({
    ai_team_rules: ["read", "update"],
  });
  const box = await screen.findByRole("textbox", {
    name: englishCopy["teamRules.label"],
  });
  await waitFor(() => expect(box).toBeEnabled());
  await user.type(box, " Mine.");
  server.rules = { ...theirs };
  server.skew = true;
  act(() => client.setQueryData(["ai-team-rules"], { ...theirs }));
  expect(
    await screen.findByText(/A colleague saved the team rules/),
  ).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        text: "Never quote a discount. Mine.",
        expected_updated_at: "2026-11-20T10:00:00Z",
      },
    ]),
  );
  expect(
    screen.getByText(/A colleague saved the team rules/),
  ).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Discard" }));
  await waitFor(() => expect(box).toHaveValue("Their text."));
  expect(screen.queryByText(/A colleague saved the team rules/)).toBeNull();
});

it("shows the conflict after a refused save, and Keep my text saves over the fresh version", async () => {
  const user = userEvent.setup();
  const { writes, server } = mountTeamRules({
    ai_team_rules: ["read", "update"],
  });
  const box = await screen.findByRole("textbox", {
    name: englishCopy["teamRules.label"],
  });
  await waitFor(() => expect(box).toBeEnabled());
  await user.type(box, " Mine.");
  server.rules = { ...theirs };
  server.skew = true;
  await user.click(screen.getByRole("button", { name: "Save" }));
  expect(
    await screen.findByText(/A colleague saved the team rules/),
  ).toBeInTheDocument();

  server.skew = false;
  await user.click(
    screen.getByRole("button", { name: englishCopy["teamRules.keepMine"] }),
  );
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(writes.at(-1)).toEqual({
      text: "Never quote a discount. Mine.",
      expected_updated_at: "2026-11-21T09:00:00Z",
    }),
  );
});
