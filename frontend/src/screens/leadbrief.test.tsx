/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { LeadBrief } from "./leadbrief";

// A lead earns no "<name> · 360" claim of authorship: leadStanding reads the
// record's own facts live, on every render, rather than a model's dated
// prose. So this card states its own plain title and carries neither a
// "Last update" line nor a WrittenBy badge, unlike the company and deal call
// it otherwise shares a shape with.

afterEach(() => {
  cleanup();
});

describe("LeadBrief", () => {
  it("names itself plainly, with no dated claim of authorship", () => {
    render(
      <LocaleProvider initial="en">
        <LeadBrief
          standing={{ label: "Your move", tone: "accent" }}
          because="No response to this lead yet."
        />
      </LocaleProvider>,
    );
    expect(screen.getByRole("heading", { name: "Lead brief" })).toBeTruthy();
    expect(screen.getByText("Your move")).toBeTruthy();
    expect(screen.getByText("No response to this lead yet.")).toBeTruthy();
    expect(screen.queryByText(/Last update/)).toBeNull();
  });

  it("draws its own thread as the caller's children", () => {
    render(
      <LocaleProvider initial="en">
        <LeadBrief standing={{ label: "Your move", tone: "accent" }}>
          <p>the lead's own thread</p>
        </LeadBrief>
      </LocaleProvider>,
    );
    expect(screen.getByText("the lead's own thread")).toBeTruthy();
  });

  it("draws no verdict when the caller has no standing to show yet", () => {
    render(
      <LocaleProvider initial="en">
        <LeadBrief />
      </LocaleProvider>,
    );
    expect(screen.getByRole("heading", { name: "Lead brief" })).toBeTruthy();
    expect(screen.queryByText("Your move")).toBeNull();
  });
});
