// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { AiCertificationCard } from "./ai-certification";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

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
  jobs: [job({ task: "draft_reply", result: "no_model", model: undefined })],
};

const CAVEATS: Certification = {
  binding_state: "bound",
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

// Stubbed at the FETCH boundary, not at the query client: the card owns its own
// queryFn, so a default queryFn on the client never reaches it, and it gates on
// useCan("ai_routing","read") which reads GET /me — unstubbed, that fails closed
// and the card renders nothing at all. Both mistakes render an empty surface
// that the story-render gate cannot tell from a working one.
function fixture(cert: Certification) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { ai_routing: ["read"] } })),
      "GET /ai/certification": () => jsonResponse(cert),
    });
    return (
      <StoryProviders>
        <AiCertificationCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AiCertificationCard> = {
  title: "Settings/Admin/AI certification",
  component: AiCertificationCard,
};
export default meta;

type Story = StoryObj<typeof AiCertificationCard>;

export const EveryResult: Story = {
  render: fixture(EVERY_RESULT),
};

// The result states are carried by Badge tones that are color-mix() derived and
// can flatten against the dark panel, so the palette is verified in both themes
// the way every other AI screen story does it.
export const EveryResultDark: Story = {
  render: fixture(EVERY_RESULT),
  globals: { theme: "dark" },
};

export const NothingBound: Story = {
  render: fixture(NOTHING_BOUND),
};

export const Caveats: Story = {
  render: fixture(CAVEATS),
};
