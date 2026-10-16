// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

// An allowance edit as typed, and what of it a save sends.

export type Allowances = components["schemas"]["AgentAllowanceSettings"];
export type Counts =
  | "agent_writes_per_window"
  | "agent_egress_per_window"
  | "agent_calls_per_window";
export type Draft = Readonly<Record<Counts | "agent_window_hours", string>>;

// The bounds the API refuses past, mirrored so a value out of range is refused
// in the box. Keyed by the wire property, which is how
// backend/gates/settingbounds_test.go holds each to the contract.
export const AGENT_ALLOWANCE_BOUNDS = {
  agent_writes_per_window: { min: 10, max: 5_000 },
  agent_egress_per_window: { min: 1, max: 200 },
  agent_calls_per_window: { min: 50, max: 20_000 },
  agent_window_hours: { min: 1, max: 168 },
} as const satisfies Record<
  Counts | "agent_window_hours",
  { min: number; max: number }
>;

export const COUNTS = [
  { property: "agent_writes_per_window", copy: "writes", release: true },
  { property: "agent_egress_per_window", copy: "egress", release: false },
  { property: "agent_calls_per_window", copy: "calls", release: false },
] as const satisfies readonly {
  property: Counts;
  copy: string;
  release: boolean;
}[];

// The window field shows the length the admin chose, waiting or not. Choosing
// the length in force again is then an edit, and it cancels the waiting change.
export function chosenWindow(allowances: Allowances): number {
  return allowances.agent_window_next_hours ?? allowances.agent_window_hours;
}

export function draftOf(allowances: Allowances): Draft {
  return {
    agent_writes_per_window: String(allowances.agent_writes_per_window),
    agent_egress_per_window: String(allowances.agent_egress_per_window),
    agent_calls_per_window: String(allowances.agent_calls_per_window),
    agent_window_hours: String(chosenWindow(allowances)),
  };
}

// A locked value keeps what is in force: a boot source owns it, and sending its
// default would be refused.
export function defaultsDraft(
  allowances: Allowances,
  locked: (property: keyof Draft) => boolean,
): Draft {
  const current = draftOf(allowances);
  const defaults = allowances.defaults;
  const pick = (property: keyof Draft) =>
    locked(property) ? current[property] : String(defaults[property]);
  return {
    agent_writes_per_window: pick("agent_writes_per_window"),
    agent_egress_per_window: pick("agent_egress_per_window"),
    agent_calls_per_window: pick("agent_calls_per_window"),
    agent_window_hours: pick("agent_window_hours"),
  };
}

export type Parsed = Readonly<
  Record<Counts | "agent_window_hours", number | null>
>;

// Digits only, inside the bounds the API refuses past: `Number` would also
// read "1e3" and " -5 " as numbers.
export function parseDraft(draft: Draft): Parsed {
  const read = (property: keyof Draft): number | null => {
    const text = draft[property].trim();
    if (!/^\d+$/.test(text)) {
      return null;
    }
    const { min, max } = AGENT_ALLOWANCE_BOUNDS[property];
    const value = Number(text);
    return value >= min && value <= max ? value : null;
  };
  return {
    agent_writes_per_window: read("agent_writes_per_window"),
    agent_egress_per_window: read("agent_egress_per_window"),
    agent_calls_per_window: read("agent_calls_per_window"),
    agent_window_hours: read("agent_window_hours"),
  };
}

export function hasRefusal(parsed: Parsed): boolean {
  return Object.values(parsed).some((value) => value === null);
}

// Only what moved is sent, so an untouched count leaves no audit entry.
export function changesOf(
  allowances: Allowances,
  parsed: Parsed,
): Partial<Record<Counts | "agent_window_hours", number>> {
  const patch: Partial<Record<Counts | "agent_window_hours", number>> = {};
  for (const row of COUNTS) {
    const next = parsed[row.property];
    if (next !== null && next !== allowances[row.property]) {
      patch[row.property] = next;
    }
  }
  const hours = parsed.agent_window_hours;
  if (hours !== null && hours !== chosenWindow(allowances)) {
    patch.agent_window_hours = hours;
  }
  return patch;
}
