// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { routeHash } from "../../lib/router";
import { searchHitRoute } from "../../lib/searchkinds";
import { NearMatches } from "./searchnearmatches";

afterEach(cleanup);

const show = (matches?: Parameters<typeof NearMatches>[0]["matches"]) =>
  render(
    <LocaleProvider initial="en">
      <NearMatches matches={matches} />
    </LocaleProvider>,
  );

describe("NearMatches", () => {
  it("draws nothing without near matches", () => {
    const { container } = show(undefined);
    expect(container.textContent).toBe("");
    cleanup();
    expect(show([]).container.textContent).toBe("");
  });

  it("links each name to its record and shows a lead's company", () => {
    show([
      { type: "contact", id: "c1", title: "Anna Kowalczyk" },
      { type: "lead", id: "l1", title: "Ida Fenn", snippet: "Brandtmeier" },
    ]);
    expect(screen.getByText("Did you mean:")).toBeTruthy();
    const route = searchHitRoute("contact", "c1");
    if (!route) {
      throw new Error("a contact has a route");
    }
    expect(
      screen.getByRole("link", { name: "Anna Kowalczyk" }).getAttribute("href"),
    ).toBe(routeHash(route));
    expect(screen.getByText(/Brandtmeier/)).toBeTruthy();
  });
});
