// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { composeStories } from "@storybook/react-vite";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { footerFinding } from "./systemhealth.fixtures";
import { readHealthReport } from "./systemhealth.report";
import * as stories from "./systemhealthfinding.stories";

const { FooterIncomplete } = composeStories(stories);

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

describe("the missing company details finding panel", () => {
  it("names the entity, the missing details and the held sends", async () => {
    render(<FooterIncomplete />);
    expect(
      await screen.findByText(
        "Beispiel GmbH: mail footers lack company details",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Privacy contact, How to object")).toBeTruthy();
    expect(
      screen.getByText("Sends by users held since the last change")
        .nextElementSibling?.textContent,
    ).toBe("2");
    expect(
      screen.getByText("Background sends held or dropped since the last change")
        .nextElementSibling?.textContent,
    ).toBe("1");
  });

  it("opens the page where the details are filled in", async () => {
    render(<FooterIncomplete />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Open Jurisdiction" }),
    );
    expect(window.location.hash).toBe("#/settings/jurisdiction");
  });

  it("copies a report of the gap it shows", async () => {
    const report = await readHealthReport(
      footerFinding,
      new AbortController().signal,
    );
    expect(JSON.parse(report)).toEqual(footerFinding.footer_gap);
  });
});
