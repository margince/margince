// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiCertificationCard } from "./ai-certification";

// What this card must never do is smooth a measurement over. Each test below is
// one honesty rule from the design: the verdict a status earns, the counts that
// make the verdict checkable, the caveats that keep a "reliable" from claiming
// more than was measured, and the two absences that are choices rather than
// gaps.

type Certification = components["schemas"]["AiCertification"];
type Job = components["schemas"]["AiCertificationJob"];

function job(over: Partial<Job>): Job {
  return {
    task: "draft_reply",
    result: "reliable",
    model: "openai/gpt-oss-120b",
    provider: "openai_compatible",
    runs: 9,
    passed: 9,
    measured_examples: 3,
    pending_examples: 0,
    scope: "full_invocation",
    sites: [],
    ...over,
  };
}

function renderCard(cert: Certification) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      const body =
        key === "GET /me"
          ? meFixture({ allow: { ai_routing: ["read"] } })
          : cert;
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui: ReactNode = (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AiCertificationCard />
      </LocaleProvider>
    </QueryClientProvider>
  );
  return render(ui);
}

function cert(jobs: Job[], over: Partial<Certification> = {}): Certification {
  return { binding_state: "bound", jobs, ...over };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("how well the AI performs", () => {
  it("gives each verdict its own words, in the reader's language", async () => {
    renderCard(
      cert([
        job({ task: "capture_classify", result: "reliable" }),
        job({ task: "draft_reply", result: "mostly_reliable" }),
        job({ task: "site_extract", result: "not_reliable" }),
        job({ task: "cold_start", result: "partly_checked" }),
        job({ task: "summarize", result: "out_of_date" }),
        job({ task: "enrich", result: "not_checked" }),
        job({ task: "offer_draft", result: "no_model" }),
      ]),
    );

    expect(await screen.findByText("Reliable")).toBeInTheDocument();
    for (const word of [
      "Mostly reliable",
      "Not reliable enough",
      "Partly checked",
      "Out of date",
      "Not checked yet",
      "No model selected",
    ]) {
      expect(screen.getByText(word)).toBeInTheDocument();
    }
    // The jobs are named in plain words, never by their contract identifier.
    expect(screen.getByText("Sorting incoming mail")).toBeInTheDocument();
    expect(screen.queryByText(/capture_classify/)).not.toBeInTheDocument();
  });

  it("does not claim an example failed when every run passed", async () => {
    // A verdict is not the pass rate alone: certification also requires the
    // grader's median score to clear a bar, so a job can pass every run and
    // still fail. draft_reply does exactly that — 12 of 12, not_supported —
    // and telling a reader "one kind of example fails every time" about it
    // would be false.
    renderCard(
      cert([
        job({
          task: "draft_reply",
          result: "not_reliable",
          runs: 12,
          passed: 12,
        }),
      ]),
    );

    expect(await screen.findByText("Not reliable enough")).toBeInTheDocument();
    expect(
      screen.getByText(/scored the answers below the bar/),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/one kind of example fails every time/),
    ).not.toBeInTheDocument();
  });

  it("shows counts rather than a rate, and says why a high score can still fail", async () => {
    // The verdict folds to the worst example, so 23 of 24 runs can pass and the
    // job still be unreliable. "96%" beside "not reliable enough" reads to a
    // person as a contradiction, so the row carries the counts and the reason.
    renderCard(
      cert([
        job({
          task: "draft_reply",
          result: "not_reliable",
          runs: 24,
          passed: 23,
        }),
      ]),
    );

    expect(
      await screen.findByText(/23 of 24 test runs passed/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/one kind of example fails every time/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/96\s*%/)).not.toBeInTheDocument();
  });

  it("says what an out-of-date measurement FOUND, not just that it is old", async () => {
    // "Out of date" describes the measurement's standing, not its finding. Two
    // committed rows are stale not_supported at 12 of 12 runs passed, so a card
    // that kept only the counts would report "do not trust this" as an old
    // perfect score.
    renderCard(
      cert([
        job({
          task: "draft_reply",
          result: "out_of_date",
          measured_result: "not_reliable",
          runs: 12,
          passed: 12,
          measured_at: "2026-08-12T00:00:00Z",
        }),
      ]),
    );

    expect(await screen.findByText("Out of date")).toBeInTheDocument();
    expect(
      screen.getByText(/when it was measured it read: Not reliable enough/),
    ).toBeInTheDocument();
  });

  it("qualifies any scope that is not full coverage", async () => {
    // The allowlist is the ONE word meaning full coverage. Allowlisting the
    // narrow words instead fails open: a scope word added to the task contract
    // would render with no caveat and read as fully checked.
    renderCard(
      cert([
        job({
          task: "agent_loop",
          result: "reliable",
          scope: "some_new_scope",
        }),
      ]),
    );

    expect(await screen.findByText("Reliable")).toBeInTheDocument();
    expect(
      screen.getByText(/only part of the job was checked/),
    ).toBeInTheDocument();
  });

  it("breaks a multi-part job into its parts in the modal", async () => {
    renderCard(
      cert([
        job({
          task: "cold_start",
          result: "not_reliable",
          worst_site: "acts",
          sites: [
            { site: "acts", result: "not_reliable", runs: 9, passed: 6 },
            { site: "company_message", result: "reliable", runs: 9, passed: 9 },
          ],
        }),
      ]),
    );

    await userEvent.click(
      await screen.findByRole("button", { name: /what do these mean/i }),
    );
    expect(
      screen.getByText(/the setup conversation — Not reliable enough/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/asking about your company — Reliable/),
    ).toBeInTheDocument();
  });

  it("keeps an out-of-date measurement visible, with its age beside it", async () => {
    // Hiding a real 88% behind "not checked" is the dishonest option, not the
    // cautious one.
    renderCard(
      cert([
        job({
          task: "site_extract",
          result: "out_of_date",
          measured_at: "2026-08-12T00:00:00Z",
        }),
      ]),
    );

    expect(await screen.findByText("Out of date")).toBeInTheDocument();
    expect(
      screen.getByText(/instructions or examples have changed since/),
    ).toBeInTheDocument();
  });

  it("says how much of a partly-checked job has actually been run", async () => {
    renderCard(
      cert([
        job({
          task: "cold_start",
          result: "partly_checked",
          measured_examples: 12,
          pending_examples: 5,
        }),
      ]),
    );

    expect(
      await screen.findByText(/12 of 17 examples checked/),
    ).toBeInTheDocument();
  });

  it("qualifies a reliable job whose run covered only part of it", async () => {
    renderCard(
      cert([
        job({ task: "agent_loop", result: "reliable", scope: "single_turn" }),
      ]),
    );

    expect(await screen.findByText("Reliable")).toBeInTheDocument();
    expect(
      screen.getByText(/only part of the job was checked/),
    ).toBeInTheDocument();
  });

  it("names an ungraded fallback without demoting the job", async () => {
    renderCard(
      cert([
        job({
          task: "draft_reply",
          result: "reliable",
          unmeasured_fallbacks: ["mistralai/ministral-8b-2512"],
        }),
      ]),
    );

    expect(await screen.findByText("Reliable")).toBeInTheDocument();
    expect(
      screen.getByText(/can fall back to mistralai\/ministral-8b-2512/),
    ).toBeInTheDocument();
  });

  it("names a measurement from another hosting setup as not carrying over", async () => {
    renderCard(
      cert([
        job({
          task: "draft_reply",
          result: "not_checked",
          measured_under_other_profile: true,
        }),
      ]),
    );

    expect(await screen.findByText("Not checked yet")).toBeInTheDocument();
    expect(
      screen.getByText(/different hosting setup, which does not carry over/),
    ).toBeInTheDocument();
  });

  it("names the site that set a job's verdict", async () => {
    renderCard(
      cert([
        job({
          task: "cold_start",
          result: "not_reliable",
          worst_site: "sitereadmessage",
        }),
      ]),
    );

    expect(
      await screen.findByText(/set by: asking about your website/),
    ).toBeInTheDocument();
  });

  it("reports an unbound installation as a choice, not as a missing measurement", async () => {
    // A reader told "not checked" would go looking for a run that would not
    // help them; what they need is to pick a model.
    renderCard(cert([job({})], { binding_state: "unbound" }));

    expect(
      await screen.findByText(/no models are bound yet/),
    ).toBeInTheDocument();
    expect(screen.queryByText("Not checked yet")).not.toBeInTheDocument();
  });

  it("counts a job it cannot name rather than printing its identifier", async () => {
    // Version skew: the server ships a job this build has no wording for.
    // Printing `some_new_job` at a reader defeats the point of the card.
    renderCard(
      cert([
        job({ task: "draft_reply" }),
        job({ task: "some_new_job" }),
        job({ task: "another_new_job" }),
      ]),
    );

    expect(
      await screen.findByText(
        /2 newer job\(s\) this version of the app cannot name/,
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/some_new_job/)).not.toBeInTheDocument();
  });

  it("explains what the check is, how it runs and what a result means", async () => {
    renderCard(cert([job({})]));

    await userEvent.click(
      await screen.findByRole("button", { name: /what do these mean/i }),
    );
    expect(
      screen.getByText(
        /fixed set of realistic examples kept alongside the code/,
      ),
    ).toBeInTheDocument();
    // The sample size is stated, because "20 of 21" means different things at
    // three runs per example and at thirty.
    expect(
      screen.getByText(/run several times through the model/),
    ).toBeInTheDocument();
    expect(screen.getByText(/safe to leave unattended/)).toBeInTheDocument();
  });

  it("shows nothing at all to a seat that may not see the binding", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(meFixture({})), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <AiCertificationCard />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(
      screen.queryByText("How well the AI performs"),
    ).not.toBeInTheDocument();
  });
});
