// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// One mark for who may read a thing. What these hold is the claim the mark
// makes to a reader: the word is exact, the look says the SHAPE of the audience
// (open, sealed, not yours), and the verb sits on the same line as the fact.

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { type Visibility, VisibilityBadge, VisibilityLine } from "./visibility";

afterEach(cleanup);

function draw(node: ReactNode) {
  return render(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

describe("VisibilityBadge", () => {
  it.each<[Visibility, string, string]>([
    ["team", "Team", "visibility-open"],
    ["participants", "Participants", "visibility-limited"],
    ["selected", "Selected", "visibility-limited"],
    ["private", "Only you", "visibility-limited"],
    ["withheld", "Withheld", "visibility-withheld"],
  ])("says %s as %j and draws it %s", (state, word, look) => {
    const { container } = draw(<VisibilityBadge state={state} />);
    // The word is the exact one, so a row and the drawer cannot disagree
    // about what to call the same message.
    expect(screen.getByText(word)).toBeInTheDocument();
    // Three looks for five states: every limit is drawn the same heavier way,
    // because the exception in a list is what a reader scans for.
    expect(container.querySelector(".visibility")?.className).toContain(look);
  });

  it("hides the icon from a screen reader, which hears the word", () => {
    const { container } = draw(<VisibilityBadge state="participants" />);
    expect(container.querySelector("svg")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
  });
});

describe("VisibilityLine", () => {
  it("seats the verb on the same line as the fact it changes", () => {
    const { container } = draw(
      <VisibilityLine
        state="team"
        action={<Button small>Make private</Button>}
      />,
    );
    const line = container.querySelector(".visibility-line");
    expect(line?.querySelector(".visibility")).toBeInTheDocument();
    expect(line?.querySelector("button")).toHaveTextContent("Make private");
  });

  it("draws no slot for a verb the reader does not have", () => {
    const { container } = draw(<VisibilityLine state="selected" />);
    // An empty slot at the far end of the line would read as a control that
    // failed to render, which is a different claim from "not yours to change".
    expect(container.querySelector(".visibility-line__action")).toBeNull();
  });

  it("keeps a second mark on the badge's side of the line", () => {
    const { container } = draw(
      <VisibilityLine
        state="participants"
        marks={<span className="reason">Marked confidential</span>}
        action={<Button small>Change visibility</Button>}
      />,
    );
    const children = Array.from(
      container.querySelector(".visibility-line")?.children ?? [],
    ).map((child) => child.className);
    // Fact, reason, then the verb — the reason explains the mark, so it reads
    // next to it rather than next to the button.
    expect(children).toEqual([
      "visibility visibility-limited",
      "reason",
      "visibility-line__action",
    ]);
  });
});
