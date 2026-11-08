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

export type RoleAllowance = components["schemas"]["AgentRoleAllowance"];
export type RoleAllowanceSet = components["schemas"]["AgentRoleAllowanceSet"];

// One role's counts as typed. An empty box uses the installation value.
export type RoleDraft = Readonly<Record<Counts, string>>;
export type RoleDrafts = Readonly<Record<string, RoleDraft>>;

// A count the role inherits starts empty; its own value or the Admin preset
// starts typed, so the box shows what binds.
export function roleDraftOf(role: RoleAllowance): RoleDraft {
  const box = (property: Counts) =>
    role[property].source === "installation"
      ? ""
      : String(role[property].value);
  return {
    agent_writes_per_window: box("agent_writes_per_window"),
    agent_egress_per_window: box("agent_egress_per_window"),
    agent_calls_per_window: box("agent_calls_per_window"),
  };
}

export function roleDraftsOf(roles: readonly RoleAllowance[]): RoleDrafts {
  return Object.fromEntries(
    roles.map((role) => [role.role_key, roleDraftOf(role)]),
  );
}

export const EMPTY_ROLE_DRAFT: RoleDraft = {
  agent_writes_per_window: "",
  agent_egress_per_window: "",
  agent_calls_per_window: "",
};

// The Admin row as the preset fills it, for "Use defaults".
export function adminPresetDraft(allowances: Allowances): RoleDraft {
  return {
    agent_writes_per_window: String(
      allowances.defaults.admin_writes_per_window,
    ),
    agent_egress_per_window: "",
    agent_calls_per_window: String(allowances.defaults.admin_calls_per_window),
  };
}

export type ParsedRole = Readonly<Record<Counts, number | null>>;

// An empty box is 0, the installation value; anything else is digits inside
// the installation's own bounds.
export function parseRoleDraft(draft: RoleDraft): ParsedRole {
  const read = (property: Counts): number | null => {
    const text = draft[property].trim();
    if (text === "") {
      return 0;
    }
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
  };
}

const sameRoleDraft = (a: RoleDraft, b: RoleDraft) =>
  COUNTS.every(({ property }) => a[property].trim() === b[property].trim());

// What a save sends for the roles: nothing while no row moved, so the preset
// and every role's values stay as they are. Once one did, the save replaces
// them all, so it carries every role with a value. Admin at the
// preset is left out and stays on it; Admin emptied is sent as zeros, which
// keeps the installation value.
export function roleChangesOf(
  allowances: Allowances,
  drafts: RoleDrafts,
): RoleAllowanceSet[] | undefined {
  const moved = allowances.roles.some(
    (role) =>
      !sameRoleDraft(
        drafts[role.role_key] ?? roleDraftOf(role),
        roleDraftOf(role),
      ),
  );
  if (!moved) {
    return undefined;
  }
  const out: RoleAllowanceSet[] = [];
  for (const role of allowances.roles) {
    const draft = drafts[role.role_key] ?? roleDraftOf(role);
    const holdsPreset = role.holds_preset;
    if (holdsPreset && sameRoleDraft(draft, adminPresetDraft(allowances))) {
      continue;
    }
    if (!holdsPreset && sameRoleDraft(draft, EMPTY_ROLE_DRAFT)) {
      continue;
    }
    const parsed = parseRoleDraft(draft);
    out.push({
      role_key: role.role_key,
      agent_writes_per_window: parsed.agent_writes_per_window ?? 0,
      agent_egress_per_window: parsed.agent_egress_per_window ?? 0,
      agent_calls_per_window: parsed.agent_calls_per_window ?? 0,
    });
  }
  return out;
}

export function hasRoleRefusal(drafts: RoleDrafts): boolean {
  return Object.values(drafts).some((draft) =>
    Object.values(parseRoleDraft(draft)).some((value) => value === null),
  );
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
