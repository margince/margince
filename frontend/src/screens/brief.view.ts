// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UrlParams } from "../app/urlstate";

// WHICH BRIEF the reader is looking at, and whose.
//
// TWO DIALS, and they are addressable — decision 2 of the plan. The Worklist
// deliberately keeps its four dials in state, and its own comment says why: an
// address carrying one of four would describe a fraction of what is on screen.
// The Brief is the case that reasoning excluded. It has exactly two, both are
// on screen at once, and it is a destination people return to and send to each
// other, so an address that could not say which half they meant would be a link
// to the wrong page half the time.
//
// DECISION 5 BINDS HERE: scope availability is per VIEW, not global. A dial
// combination the product cannot answer must never be selectable, so
// `scopesFor` is what the control reads rather than a flat list — a Team dial
// offered on a view with no team surface behind it is a control that resolves
// to nothing.

/** Morning is what waits today; Weekly is the week closed and the week ahead. */
export const VIEWS = ["morning", "weekly"] as const;
export type BriefView = (typeof VIEWS)[number];

/** Whose Brief: the reader's own, or the team they lead. */
export const SCOPES = ["mine", "team"] as const;
export type BriefScope = (typeof SCOPES)[number];

export type BriefAddress = Readonly<{ view: BriefView; scope: BriefScope }>;

/** The address a reader who has chosen nothing is at. */
export const DEFAULT_ADDRESS: BriefAddress = { view: "morning", scope: "mine" };

const VIEW_PARAM = "view";
const SCOPE_PARAM = "scope";

/**
 * Which scopes this view can actually answer.
 *
 * BOTH views have a team surface today — the team board on Morning, the frozen
 * team week on Weekly — but only for a reader whose row scope reaches a team,
 * which is `offered`. A rep gets one scope and therefore no dial at all: a
 * control with one option asks a reader to confirm what they cannot change.
 */
export function scopesFor(
  _view: BriefView,
  offered: boolean,
): readonly BriefScope[] {
  return offered ? SCOPES : ["mine"];
}

/**
 * Read the address, narrowed to what the product can answer.
 *
 * An unknown or unreachable value falls back rather than erroring: an address
 * is something a person can type, a link can carry from an older build, and a
 * colleague can send from a seat with wider reach than the reader's. None of
 * those should produce a broken page — they should produce the nearest page
 * this reader is entitled to.
 */
export function addressFrom(params: UrlParams, offered: boolean): BriefAddress {
  const view = asView(params.get(VIEW_PARAM));
  const asked = asScope(params.get(SCOPE_PARAM));
  // NARROWED BY WHAT THIS READER MAY SEE, not only by the vocabulary. A link to
  // `?scope=team` from a lead, opened by a rep, resolves to their own Brief —
  // the server would refuse the team read anyway, and a dial stuck on a scope
  // the page cannot fill is a page that looks broken.
  const reachable = scopesFor(view, offered).includes(asked);
  return { view, scope: reachable ? asked : DEFAULT_ADDRESS.scope };
}

/**
 * Write the address, omitting whatever is already the default.
 *
 * `#/home` and `#/home?view=morning&scope=mine` are the same page, and the
 * shorter one is what a reader who has changed nothing should be able to copy.
 * The writer sorts its keys (hashWithParams), so the same state is always the
 * same string.
 */
export function paramsFor(address: BriefAddress): UrlParams {
  const params = new Map<string, string>();
  if (address.view !== DEFAULT_ADDRESS.view) {
    params.set(VIEW_PARAM, address.view);
  }
  if (address.scope !== DEFAULT_ADDRESS.scope) {
    params.set(SCOPE_PARAM, address.scope);
  }
  return params;
}

function asView(raw: string | undefined): BriefView {
  return VIEWS.find((view) => view === raw) ?? DEFAULT_ADDRESS.view;
}

function asScope(raw: string | undefined): BriefScope {
  return SCOPES.find((scope) => scope === raw) ?? DEFAULT_ADDRESS.scope;
}
