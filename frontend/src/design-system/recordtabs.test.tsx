// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { RecordTabs } from "./recordtabs";

afterEach(cleanup);

type Body = "overview" | "contacts";
const LABELS: Record<Body, string> = {
  overview: "Overview",
  contacts: "Contacts",
};

function strip(trailing?: React.ReactNode) {
  return render(
    <LocaleProvider initial="en">
      <RecordTabs
        options={["overview", "contacts"]}
        value="overview"
        onChange={() => undefined}
        labels={LABELS}
        trailing={trailing}
      />
    </LocaleProvider>,
  );
}

// The row's far end is the page's control, not one more tab: it sits outside
// the strip a reader navigates, and a strip with nothing to put there draws
// no empty end.
describe("RecordTabs carries the details control at the row's end", () => {
  it("renders the trailing control outside the strip", () => {
    const { container } = strip(<Button>Details</Button>);
    const trailing = container.querySelector(".recordtabs-trailing");
    expect(trailing).not.toBeNull();
    expect(
      trailing?.contains(screen.getByRole("button", { name: "Details" })),
    ).toBe(true);
    expect(
      container.querySelector(".recordtabs-strip")?.contains(trailing),
    ).toBe(false);
  });

  it("draws no end when there is nothing to put there", () => {
    const { container } = strip();
    expect(container.querySelector(".recordtabs-trailing")).toBeNull();
  });
});

// A record read in ONE body has nothing to choose, so its lone tab is a label
// for the page the reader is already on. Drawn as a button it was a control
// that answered nothing: pressable, focusable, and dead. It keeps the tab's
// box and the current tab's mark, because the row still has to read as the row.
describe("RecordTabs draws a lone body as a label", () => {
  function lone() {
    return render(
      <LocaleProvider initial="en">
        <RecordTabs
          options={["overview"]}
          value="overview"
          labels={LABELS}
          trailing={<Button>Details</Button>}
        />
      </LocaleProvider>,
    );
  }

  it("puts no pressable tab in the strip", () => {
    const { container } = lone();
    const tabs = container.querySelectorAll(".recordtabs-tab");
    expect(tabs).toHaveLength(1);
    expect(tabs[0]?.tagName).toBe("SPAN");
    expect(container.querySelector(".recordtabs-strip button")).toBeNull();
  });

  it("marks the label as the page the reader is on", () => {
    const { container } = lone();
    const tab = container.querySelector(".recordtabs-tab");
    expect(tab?.textContent).toBe("Overview");
    expect(tab?.getAttribute("aria-current")).toBe("page");
    expect(tab?.hasAttribute("aria-pressed")).toBe(false);
    expect(tab?.getAttribute("role")).toBeNull();
  });

  it("still carries the control at the row's end", () => {
    const { container } = lone();
    expect(
      container
        .querySelector(".recordtabs-trailing")
        ?.contains(screen.getByRole("button", { name: "Details" })),
    ).toBe(true);
  });
});

// More than one body and the tabs are what they were: buttons that route, with
// the current one pressed.
describe("RecordTabs keeps pressable tabs when there is a choice", () => {
  it("draws a button per option and presses the current one", () => {
    const { container } = strip();
    const tabs = container.querySelectorAll(".recordtabs-tab");
    expect(tabs).toHaveLength(2);
    for (const tab of tabs) {
      expect(tab.tagName).toBe("BUTTON");
      expect(tab.hasAttribute("aria-current")).toBe(false);
    }
    expect(
      screen
        .getByRole("button", { name: "Overview" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen
        .getByRole("button", { name: "Contacts" })
        .getAttribute("aria-pressed"),
    ).toBe("false");
  });
});
