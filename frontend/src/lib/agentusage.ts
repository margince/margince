// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export type AgentUsage = components["schemas"]["AgentUsage"];
export type AgentCounterUsage = components["schemas"]["AgentCounterUsage"];
export type AgentUsageState = components["schemas"]["AgentUsageState"];

// The three allowances, in the order every usage view lists them.
export const AGENT_COUNTERS = ["writes", "egress", "calls"] as const;
export type AgentCounter = (typeof AGENT_COUNTERS)[number];

type CounterSet = Readonly<Record<AgentCounter, AgentCounterUsage>>;

function share(counter: AgentCounterUsage): number {
  return counter.limit > 0 ? counter.used / counter.limit : 0;
}

// The allowance closest to running out, which is the one a row has room to
// show. The first in list order wins a tie.
export function tightestCounter(usage: CounterSet): AgentCounter {
  return AGENT_COUNTERS.reduce((tightest, counter) =>
    share(usage[counter]) > share(usage[tightest]) ? counter : tightest,
  );
}

// A used-up allowance is drawn in danger. Nearness is the server's call
// (`state`), so a bar short of its limit keeps the plain fill.
export function usedUp(counter: AgentCounterUsage): boolean {
  return counter.limit > 0 && counter.used >= counter.limit;
}
