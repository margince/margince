// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { REPORTING_FIXTURE_ZONE } from "../testing/reporting.fixtures";
import { readsTeamProgress } from "./progressreader";

type AnalyticsContext = components["schemas"]["AnalyticsContext"];
type Scope = AnalyticsContext["default_scope"];

const own: Scope = { kind: "owner", id: "u1", label: "Alex Brandt" };
const team: Scope = { kind: "team", id: "t1", label: "Sales DACH" };
const managed: Scope = { kind: "managed_teams", label: "My teams" };

function context(defaultScope: Scope, allowed: Scope[]): AnalyticsContext {
  return {
    default_scope: defaultScope,
    allowed_scopes: allowed,
    capabilities: {
      view_manager_forecast: false,
      submit_manager_forecast: false,
    },
    as_of: "2026-03-15T09:00:00Z",
    timezone: REPORTING_FIXTURE_ZONE,
    base_currency: "EUR",
  };
}

describe("readsTeamProgress", () => {
  it("names a manager who opens on their teams by the team", () => {
    expect(readsTeamProgress(context(managed, [own, team]))).toBe(true);
  });

  it("names a seller, who reads only their own figures", () => {
    expect(readsTeamProgress(context(own, [own]))).toBe(false);
  });

  it("names a manager who opens on their own figures by those", () => {
    expect(readsTeamProgress(context(own, [own, team]))).toBe(false);
  });

  it("names a reader by their own figures while no team is theirs to measure", () => {
    const unread: Scope = { ...team, measurable: false };
    expect(readsTeamProgress(context(managed, [own, unread]))).toBe(false);
  });

  it("names a reader by their own figures when the answer is no frame", () => {
    const page: unknown = { data: [] };
    expect(readsTeamProgress(page as AnalyticsContext)).toBe(false);
  });

  it("names a reader whose context has not arrived by their own figures", () => {
    expect(readsTeamProgress(undefined)).toBe(false);
  });
});
