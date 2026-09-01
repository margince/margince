// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { AiHealthCard } from "./ai-health";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The three states worth a picture, because they are what an operator has to
// tell apart at a glance: every lane answering, one lane down with its reason,
// and an installation nobody used this hour — which must not look like the
// second.

type RungHealth = components["schemas"]["AiRungHealth"];

const HEALTHY: RungHealth[] = [
  {
    tier: "local_small",
    healthy: true,
    calls: 42,
    failures: 0,
    median_latency_ms: 210,
    last_call_at: "2026-09-01T09:14:00Z",
  },
  {
    tier: "cloud_large",
    healthy: true,
    calls: 6,
    failures: 1,
    median_latency_ms: 1840,
    last_sentinel: "rate_limited",
    last_call_at: "2026-09-01T09:02:00Z",
  },
];

// One lane dead, which is the state the whole endpoint exists for: under the
// capture posture nothing else on the product would show it.
const OUTAGE: RungHealth[] = [
  HEALTHY[0],
  {
    tier: "cloud_large",
    healthy: false,
    calls: 9,
    failures: 9,
    median_latency_ms: 28,
    last_sentinel: "provider_unavailable",
    last_call_at: "2026-09-01T08:58:00Z",
  },
];

function story(rungs: RungHealth[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /ai/health": () => jsonResponse({ window_hours: 1, rungs }),
    });
    return (
      <StoryProviders>
        <AiHealthCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AiHealthCard> = {
  title: "Settings/Admin/Model lanes",
  component: AiHealthCard,
};
export default meta;

type Story = StoryObj<typeof AiHealthCard>;

export const Healthy: Story = { render: story(HEALTHY) };
export const Outage: Story = { render: story(OUTAGE) };
export const NobodyCalledAModel: Story = { render: story([]) };
