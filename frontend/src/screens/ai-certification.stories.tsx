// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { AiCertificationCard } from "./ai-certification";

type Certification = components["schemas"]["AiCertification"];
type Job = components["schemas"]["AiCertificationJob"];

function job(over: Partial<Job>): Job {
  return {
    task: "draft_reply",
    result: "reliable",
    provider: "openai_compatible",
    model: "openai/gpt-oss-120b",
    runs: 9,
    passed: 9,
    measured_examples: 3,
    pending_examples: 0,
    scope: "full_invocation",
    sites: [],
    ...over,
  };
}

// Every verdict at once, which is the one view that shows whether the seven
// words read as seven different claims rather than as a gradient.
const EVERY_RESULT: Certification = {
  binding_state: "bound",
  runs_per_example: 3,
  jobs: [
    job({ task: "capture_classify", runs: 21, passed: 21 }),
    job({
      task: "capture_confidentiality_verdict",
      result: "mostly_reliable",
      runs: 21,
      passed: 20,
    }),
    // The case a percentage would misreport: the verdict folds to the worst
    // example, so 23 of 24 is still not reliable enough.
    job({
      task: "capture_counterparty_verdict",
      result: "not_reliable",
      runs: 24,
      passed: 23,
    }),
    job({
      task: "cold_start",
      result: "partly_checked",
      measured_examples: 12,
      pending_examples: 5,
      worst_site: "sitereadmessage",
    }),
    job({
      task: "site_extract",
      result: "out_of_date",
      measured_at: "2026-08-12T00:00:00Z",
      runs: 15,
      passed: 13,
    }),
    job({
      task: "enrich",
      result: "not_checked",
      runs: undefined,
      passed: undefined,
      measured_under_other_profile: true,
    }),
    job({
      task: "offer_draft",
      result: "no_model",
      model: undefined,
      provider: undefined,
      runs: undefined,
      passed: undefined,
    }),
  ],
};

const NOTHING_BOUND: Certification = {
  binding_state: "unbound",
  runs_per_example: 3,
  jobs: [
    job({ task: "draft_reply", result: "no_model", model: undefined }),
  ],
};

const CAVEATS: Certification = {
  binding_state: "bound",
  runs_per_example: 3,
  jobs: [
    // Reliable, but only one turn of a multi-turn path was graded.
    job({ task: "agent_loop", scope: "single_turn" }),
    // Reliable today, with a budget-pressure fallback nobody has graded.
    job({
      task: "draft_reply",
      unmeasured_fallbacks: ["mistralai/ministral-8b-2512"],
    }),
  ],
};

function Fixture({ cert }: Readonly<{ cert: Certification }>) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, queryFn: async () => cert },
    },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 720 }}>
          <AiCertificationCard />
        </div>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

const meta: Meta<typeof AiCertificationCard> = {
  title: "Screens/AI certification",
  component: AiCertificationCard,
};
export default meta;

type Story = StoryObj<typeof AiCertificationCard>;

export const EveryResult: Story = {
  render: () => <Fixture cert={EVERY_RESULT} />,
};

export const NothingBound: Story = {
  render: () => <Fixture cert={NOTHING_BOUND} />,
};

export const Caveats: Story = {
  render: () => <Fixture cert={CAVEATS} />,
};
