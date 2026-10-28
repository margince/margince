// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { MailCoverageNotice } from "./mailcoveragenotice";

afterEach(cleanup);
type Coverage = components["schemas"]["MailCaptureCoverage"];

it.each<Coverage["state"]>(["partial", "unknown", "unavailable", "withheld"])(
  "qualifies recorded history when coverage is %s",
  (state) => {
    render(
      <LocaleProvider initial="en">
        <MailCoverageNotice
          coverage={{
            channel: "email",
            state,
            interval_to: "2026-08-24T12:00:00Z",
            evaluated_at: "2026-08-24T12:00:00Z",
          }}
        />
      </LocaleProvider>,
    );
    expect(screen.getByText("Mail history may be incomplete")).toBeVisible();
    if (state === "withheld") {
      expect(screen.queryByRole("link")).not.toBeInTheDocument();
    } else {
      expect(
        screen.getByRole("link", { name: "Check connections" }),
      ).toHaveAttribute("href", "#/settings/connections");
    }
  },
);

it("keeps an older server's absent coverage unknown", () => {
  render(
    <LocaleProvider initial="en">
      <MailCoverageNotice />
    </LocaleProvider>,
  );
  expect(
    screen.getByText(/a reply may exist outside this record/),
  ).toBeVisible();
});
