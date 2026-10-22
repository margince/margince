// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { HowItStands } from "./howitstands";

afterEach(cleanup);

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
    rerender(
      <HowItStands
        date="13 Aug"
        verdict={{ word: "Not assessed", tone: "neutral" }}
      />,
    );
    expect(sheet()?.style.getPropertyValue("--standing-wash")).toBe(
      "transparent",
    );
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
});
