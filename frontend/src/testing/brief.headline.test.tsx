/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { cleanup, render as rtlRender } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Heading } from "../design-system/heading";
import { BriefFeed } from "../features/brief/brief.feed";
import { RowName } from "../features/worklist/worklist.rowname";
import { LocaleProvider } from "../i18n";
import { legacyStalledRow, readingsDay } from "./brief.fixtures";
import { render, stubApi } from "./brief.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  document.head.replaceChildren();
});

// The stylesheets that set the headline, read as the browser gets them: the
// test environment loads no CSS on import.
const SHEETS = [
  "../design-system/base.css",
  "../features/worklist/worklist.row.css",
  "../features/brief/brief.feed.css",
].map((path) => readFileSync(resolve(__dirname, path), "utf8"));

const LONG_SUBJECT =
  "Quarterly business review with the procurement, legal and IT security teams about the renewal terms";

function loadSheets() {
  for (const css of SHEETS) {
    const sheet = document.createElement("style");
    sheet.textContent = css;
    document.head.append(sheet);
  }
}

it("wraps a heading-shaped headline inside the card", () => {
  loadSheets();
  stubApi({});
  const row: ReturnType<typeof legacyStalledRow> = {
    ...legacyStalledRow(),
    subject: { type: "deal", id: "deal-expansion", label: LONG_SUBJECT },
  };
  const { container } = render(
    <BriefFeed day={readingsDay({}, [row])} state="ready" />,
  );
  const heading = container.querySelector(
    ".brief-triage-lead .worklist-row-title > .heading",
  );
  expect(heading?.textContent).toContain(LONG_SUBJECT);
  const name = heading?.firstElementChild;
  if (!(heading instanceof HTMLElement) || !(name instanceof HTMLElement)) {
    throw new Error("the headline is not drawn as a heading");
  }
  expect(getComputedStyle(heading).minWidth).toBe("0");
  expect(getComputedStyle(name).whiteSpace).toBe("normal");
  expect(name.matches(".btn:not(.btn-valuelabel)")).toBe(false);
});

it("starts a task name drawn as a button inside the heading at the card's edge", () => {
  loadSheets();
  const { container } = rtlRender(
    <LocaleProvider initial="en">
      <div className="brief-triage-lead">
        <div className="worklist-row-title">
          <Heading size="large" as="h3">
            <RowName title={LONG_SUBJECT} open={() => undefined} />
          </Heading>
        </div>
      </div>
    </LocaleProvider>,
  );
  const name = container.querySelector(".heading > .btn");
  if (!(name instanceof HTMLElement)) {
    throw new Error("the name is not drawn as a button");
  }
  const style = getComputedStyle(name);
  expect(style.whiteSpace).toBe("normal");
  expect(style.textAlign).toBe("start");
  expect(style.justifyContent).toBe("flex-start");
});
