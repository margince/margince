/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { scopeKey, scopeQuery, writableScope } from "./analytics.context";
import { AnalyticsScopePicker } from "./analytics.scope";

afterEach(() => {
  cleanup();
});

const WORKSPACE = { kind: "workspace" as const, label: "Whole workspace" };
const NORTH = { kind: "team" as const, id: "t-north", label: "North" };
const SOUTH = { kind: "team" as const, id: "t-south", label: "South" };
const MINE = { kind: "owner" as const, id: "u-me", label: "Lena Fischer" };
const MANAGED = { kind: "managed_teams" as const, label: "My teams" };

describe("what a selected population sends", () => {
  // The whole point of the manager default: it names no single subject, so it
  // travels as an OMISSION and the server resolves the lens. Sent as a kind, it
  // would be a request for a named managed set, which is not a thing to ask for.
  it("sends a manager's own default as no scope at all", () => {
    expect(scopeQuery(MANAGED)).toEqual({});
  });

  it("names the population for every scope that has one", () => {
    expect(scopeQuery(WORKSPACE)).toEqual({ scope_kind: "workspace" });
    expect(scopeQuery(NORTH)).toEqual({
      scope_kind: "team",
      scope_id: "t-north",
    });
    expect(scopeQuery(MINE)).toEqual({ scope_kind: "owner", scope_id: "u-me" });
  });

  // A read may leave the population to the server; a write may not. A forecast
  // is an assertion about ONE named population, and a manager's default covers
  // several — so there is nothing honest to write it against.
  it("refuses to name a write population the reader has not chosen", () => {
    expect(writableScope(MANAGED)).toBeNull();
    expect(writableScope(NORTH)).toEqual({
      scope_kind: "team",
      scope_id: "t-north",
    });
  });

  // Two teams renamed to the same words are still two populations. A key built
  // from the label would collapse them and serve one team's numbers under the
  // other's name.
  it("keys two populations apart even when they read the same", () => {
    const renamed = { ...SOUTH, label: NORTH.label };
    expect(scopeKey(NORTH)).not.toEqual(scopeKey(renamed));
  });
});

describe("the population control", () => {
  it("offers every population the server allowed", async () => {
    const onSelect = vi.fn();
    render(
      <AnalyticsScopePicker
        scopes={[WORKSPACE, NORTH, SOUTH]}
        selected={WORKSPACE}
        onSelect={onSelect}
      />,
    );

    await userEvent.click(screen.getByRole("combobox"));
    expect(screen.getByRole("option", { name: "North" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "South" })).toBeTruthy();
  });

  it("reports the population that was picked, not its label", async () => {
    const onSelect = vi.fn();
    render(
      <AnalyticsScopePicker
        scopes={[WORKSPACE, NORTH]}
        selected={WORKSPACE}
        onSelect={onSelect}
      />,
    );

    await userEvent.click(screen.getByRole("combobox"));
    await userEvent.click(screen.getByRole("option", { name: "North" }));
    expect(onSelect).toHaveBeenCalledWith(NORTH);
  });

  // A rep measures one population, so there is no question to ask. A dropdown
  // holding a single option is a control that cannot do anything.
  it("states the population plainly when there is only one", () => {
    render(
      <AnalyticsScopePicker
        scopes={[MINE]}
        selected={MINE}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.getByText(/Lena Fischer/)).toBeTruthy();
  });
});
