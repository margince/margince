// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What each Overview view is asked about, and where a reader's own choice of
// it is kept. No React here, so the rules are read and tested on their own.

import {
  readStored,
  readStoredJson,
  removeStored,
  STORAGE_KEYS,
  type StorageKey,
  writeStored,
} from "../../lib/storage";
import type { UrlParams } from "../../lib/urlstate";
import {
  isOverviewView,
  OVERVIEW_DIAL,
  type OverviewView,
} from "./analytics.overview.choice";
import { REPORTING_PERIODS } from "./reporting.filters";

/** The address dials that make up one view's question. */
export const CONTEXT_DIALS = [
  "scope",
  "pipeline",
  "period",
  "from",
  "through",
  "close",
] as const;
type ContextDial = (typeof CONTEXT_DIALS)[number];

/**
 * One view's question as the address spells it. An absent dial means the
 * view's default, so an omitted pipeline (the default one) and `all` stay
 * two different questions.
 */
export type OverviewDials = Readonly<Partial<Record<ContextDial, string>>>;

// The periods the team review reads: a team is coached week by week, and a
// month is the longest span its handoff rounds stay comparable over.
export const TEAM_PERIODS = [
  "this_week",
  "last_week",
  "this_month",
  "last_month",
] as const;

// My prospecting always reads the reader's own work, so it keeps no scope.
const VIEW_DIALS: Readonly<Record<OverviewView, readonly ContextDial[]>> = {
  sales: CONTEXT_DIALS,
  prospecting: ["period", "from", "through"],
  team: ["scope", "period"],
};

const VIEW_PERIODS: Readonly<Record<OverviewView, readonly string[]>> = {
  sales: REPORTING_PERIODS,
  prospecting: REPORTING_PERIODS,
  team: TEAM_PERIODS,
};

/** The dials of `view` that the address names. */
export function dialsOf(params: UrlParams, view: OverviewView): OverviewDials {
  const dials: Partial<Record<ContextDial, string>> = {};
  for (const dial of VIEW_DIALS[view]) {
    const value = params.get(dial);
    if (value) dials[dial] = value;
  }
  return dials;
}

/** Whether the address states a question for `view`, and so wins. */
export function namesContext(params: UrlParams, view: OverviewView): boolean {
  return Object.keys(dialsOf(params, view)).length > 0;
}

/** `params` opened on `view` with exactly `dials` as its question. */
export function withContext(
  params: UrlParams,
  view: OverviewView,
  dials: OverviewDials,
): Map<string, string> {
  const next = new Map(params);
  for (const dial of CONTEXT_DIALS) next.delete(dial);
  for (const dial of VIEW_DIALS[view]) {
    const value = dials[dial];
    if (value) next.set(dial, value);
  }
  next.set(OVERVIEW_DIAL, view);
  return next;
}

/**
 * The remembered dials this reader may still ask. A scope they lost or a
 * period the view no longer offers is dropped, so the view falls back to its
 * default instead of asking a question the server would refuse. A pipeline
 * is kept: Sales review says itself when a pipeline it names is gone.
 */
export function permittedDials(
  dials: OverviewDials,
  view: OverviewView,
  scopes: readonly string[],
): OverviewDials {
  const kept: Partial<Record<ContextDial, string>> = {};
  for (const dial of VIEW_DIALS[view]) {
    const value = dials[dial];
    if (value && permitted(dial, value, view, scopes)) kept[dial] = value;
  }
  return kept;
}

function permitted(
  dial: ContextDial,
  value: string,
  view: OverviewView,
  scopes: readonly string[],
): boolean {
  switch (dial) {
    case "scope":
      return scopes.includes(value);
    case "period":
      return VIEW_PERIODS[view].includes(value);
    case "close":
      return value === "all_open" || value === "fiscal_quarter";
    default:
      return true;
  }
}

type Remembered = Readonly<{
  view?: OverviewView;
  contexts: Partial<Record<OverviewView, OverviewDials>>;
}>;

const NOTHING: Remembered = { contexts: {} };

// Only the shape this file writes is read back. Anything else, such as a
// value an earlier build wrote, reads as nothing remembered.
function asRemembered(value: unknown): Remembered | null {
  if (typeof value !== "object" || value === null) return null;
  const view = "view" in value ? value.view : undefined;
  const stored = "contexts" in value ? value.contexts : undefined;
  const contexts: Partial<Record<OverviewView, OverviewDials>> = {};
  if (typeof stored === "object" && stored !== null) {
    for (const [name, dials] of Object.entries(stored)) {
      if (isOverviewView(name)) contexts[name] = asDials(dials);
    }
  }
  return {
    view: typeof view === "string" && isOverviewView(view) ? view : undefined,
    contexts,
  };
}

function asDials(value: unknown): OverviewDials {
  const dials: Partial<Record<ContextDial, string>> = {};
  if (typeof value !== "object" || value === null) return dials;
  const entries = new Map(Object.entries(value));
  for (const dial of CONTEXT_DIALS) {
    const held = entries.get(dial);
    if (typeof held === "string" && held) dials[dial] = held;
  }
  return dials;
}

const chosenKey = (account: string) => ({
  family: STORAGE_KEYS.analyticsOverview,
  member: account,
});
const tabKey = (account: string) => ({
  family: STORAGE_KEYS.analyticsOverviewTab,
  member: account,
});

function read(key: StorageKey): Remembered {
  return readStoredJson(key, asRemembered) ?? NOTHING;
}

function write(
  key: StorageKey,
  change: (held: Remembered) => Remembered,
): void {
  writeStored(key, JSON.stringify(change(read(key))));
}

/**
 * The view this account chose last. A choice kept before choices were per
 * account is still honoured: a seat key is erased at sign-out, so it was made
 * by whoever is signed in now.
 */
export function rememberedView(account: string): string | null {
  return (
    read(chosenKey(account)).view ??
    readStored(STORAGE_KEYS.analyticsOverviewView)
  );
}

export function rememberView(account: string, view: OverviewView): void {
  write(chosenKey(account), (held) => ({ ...held, view }));
  removeStored(STORAGE_KEYS.analyticsOverviewView);
}

/**
 * The dials `view` opens with when the address names none: this tab's last
 * question for it first, then the account's own last choice.
 */
export function rememberedContext(
  account: string,
  view: OverviewView,
): OverviewDials | undefined {
  return (
    read(tabKey(account)).contexts[view] ??
    read(chosenKey(account)).contexts[view]
  );
}

/** What this tab showed for `view`, so a switch back finds it again. */
export function noteContext(
  account: string,
  view: OverviewView,
  dials: OverviewDials,
): void {
  write(tabKey(account), (held) => withView(held, view, dials));
}

/** The reader turned a dial: their own choice for `view` from now on. */
export function chooseContext(
  account: string,
  view: OverviewView,
  dials: OverviewDials,
): void {
  noteContext(account, view, dials);
  write(chosenKey(account), (held) => withView(held, view, dials));
}

function withView(
  held: Remembered,
  view: OverviewView,
  dials: OverviewDials,
): Remembered {
  return { ...held, contexts: { ...held.contexts, [view]: dials } };
}

/**
 * The dials a door out of a view carries: its question as answered, with the
 * defaults spelled out so the destination cannot read a different default.
 * The reader's default scope is left out, because every section reads it.
 */
export function outboundDials(
  view: OverviewView,
  answered: OverviewDials,
  defaultScope: string,
): Map<string, string> {
  const dials = new Map<string, string>([[OVERVIEW_DIAL, view]]);
  for (const dial of CONTEXT_DIALS) {
    const value = answered[dial];
    if (value && !(dial === "scope" && value === defaultScope))
      dials.set(dial, value);
  }
  return dials;
}
