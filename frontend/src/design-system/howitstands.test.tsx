// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HowItStands } from "./howitstands";

const here = dirname(fileURLToPath(import.meta.url));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("How it stands", () => {
  it("credits the CRM's records by default, never an agent", () => {
    render(
      <HowItStands
        date="13 Aug"
        verdict={{ word: "Watch", tone: "warning" }}
      />,
    );
    expect(
      screen.getByRole("heading", { name: "How it stands · 13 Aug" }),
    ).toBeInTheDocument();
    expect(screen.getByText("From CRM records")).toBeInTheDocument();
    expect(document.querySelector(".badge-ai")).toBeNull();
  });

  it("wears the indigo pill and counts sources only for a model's verdict", () => {
    render(
      <HowItStands
        date="13 Aug"
        sources={{ by: "model", count: 4 }}
        verdict={{ word: "Drifting", tone: "warning" }}
      />,
    );
    expect(screen.getByText("Margince · 4 sources")).toBeInTheDocument();
    expect(document.querySelector(".badge-ai")).not.toBeNull();
  });

  it("washes the card in the verdict's tone and leaves a neutral one plain", () => {
    const { rerender } = render(
      <HowItStands
        date="13 Aug"
        verdict={{ word: "At risk", tone: "danger" }}
      />,
    );
    const sheet = () => document.querySelector<HTMLElement>(".standing-sheet");
    expect(sheet()?.style.getPropertyValue("--standing-wash")).toBe(
      "var(--dangerBg)",
    );
    expect(sheet()?.style.getPropertyValue("--standing-edge")).toBe(
      "var(--dangerText)",
    );
    rerender(
      <HowItStands
        date="13 Aug"
        verdict={{ word: "Not assessed", tone: "neutral" }}
      />,
    );
    expect(sheet()?.style.getPropertyValue("--standing-wash")).toBe(
      "transparent",
    );
    expect(sheet()?.style.getPropertyValue("--standing-edge")).toBe("");
  });

  it("puts a ladder hero under the readings and a ring beside them", () => {
    const { rerender } = render(
      <HowItStands
        date="13 Aug"
        verdict={{ word: "Live", tone: "success" }}
        hero={<span>hero</span>}
        heroLayout="below"
        dims={<span>dims</span>}
      />,
    );
    expect(
      screen.getByText("hero").closest(".standing-sheet-readings"),
    ).toBeNull();
    rerender(
      <HowItStands
        date="13 Aug"
        verdict={{ word: "Live", tone: "success" }}
        hero={<span>hero</span>}
        dims={<span>dims</span>}
      />,
    );
    expect(
      screen.getByText("hero").closest(".standing-sheet-readings"),
    ).not.toBeNull();
  });

  it("holds a note to two lines and opens it in place when clipped", () => {
    // happy-dom lays nothing out: the note reads as taller than its box.
    vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockImplementation(
      function (this: HTMLElement) {
        return this.classList.contains("standing-sheet-note") ? 80 : 0;
      },
    );
    render(
      <HowItStands
        date="13 Aug"
        verdict={{
          word: "Lost",
          sentence: "Closed 12 Aug · Price",
          note: "The board chose to stay with their supplier.",
          tone: "neutral",
        }}
      />,
    );
    const note = screen.getByText(/stay with their supplier/);
    expect(note).toHaveAttribute("data-clamped");
    const more = screen.getByRole("button", { name: "Show more" });
    expect(more).toHaveAttribute("aria-controls", note.id);
    fireEvent.click(more);
    expect(note).not.toHaveAttribute("data-clamped");
    const less = screen.getByRole("button", { name: "Show less" });
    expect(less).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(less);
    expect(note).toHaveAttribute("data-clamped");
    expect(screen.getByRole("button", { name: "Show more" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  // happy-dom loads no stylesheet, so the two-line cut is read off the sheet.
  it("cuts a clamped note at two lines", () => {
    const css = readFileSync(join(here, "howitstands.css"), "utf8");
    const rule = /\.standing-sheet-note\[data-clamped\]\s*\{([^}]*)\}/.exec(
      css,
    );
    expect(rule?.[1]).toMatch(/(?:^|[\s;])line-clamp:\s*2;/);
    expect(rule?.[1]).toMatch(/-webkit-line-clamp:\s*2;/);
    expect(rule?.[1]).toMatch(/display:\s*-webkit-box;/);
    expect(rule?.[1]).toMatch(/-webkit-box-orient:\s*vertical;/);
    expect(rule?.[1]).toMatch(/overflow:\s*hidden;/);
  });

  it("offers no control for a note that fits", () => {
    render(
      <HowItStands
        date="13 Aug"
        verdict={{ word: "Lost", note: "Budget frozen", tone: "neutral" }}
      />,
    );
    expect(screen.getByText("Budget frozen")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
