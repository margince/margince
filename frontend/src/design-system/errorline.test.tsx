// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../screens/common";
import { Button } from "./atoms";
import { ErrorLine } from "./errorline";

afterEach(cleanup);

function inEnglish(node: ReactNode) {
  return render(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

describe("ErrorLine", () => {
  it("draws nothing while there is no error, so a query's error goes in straight", () => {
    const { container } = inEnglish(
      <>
        <ErrorLine error={null} />
        <ErrorLine error={undefined} />
      </>,
    );
    expect(container.innerHTML).toBe("");
  });

  it("announces a thrown problem in the reader's words, in the danger ink", () => {
    inEnglish(
      <ErrorLine error={new ProblemError({ detail: "email taken" })} />,
    );
    const line = screen.getByRole("alert");
    expect(line.tagName).toBe("P");
    expect(line.textContent).toBe("email taken");
    expect(line.className).toBe("t-danger");
  });

  it("never shows a bug's own message to the reader", () => {
    inEnglish(<ErrorLine error={new TypeError("x is undefined")} />);
    expect(screen.getByRole("alert").textContent).not.toContain("undefined");
  });

  it("announces a sentence the caller already translated", () => {
    inEnglish(<ErrorLine>Pick a stage first.</ErrorLine>);
    expect(screen.getByRole("alert").textContent).toBe("Pick a stage first.");
  });

  it("draws its verbs after the sentence, on the same line", () => {
    inEnglish(
      <ErrorLine actions={<Button variant="ghost">Re-read</Button>}>
        This record changed.
      </ErrorLine>,
    );
    const line = screen.getByRole("alert");
    expect(line.textContent).toBe("This record changed.Re-read");
    expect(line.querySelector("button")?.textContent).toBe("Re-read");
    expect(line.classList.contains("error-line")).toBe(true);
  });

  it("carries the id a control's aria-describedby points at", () => {
    inEnglish(<ErrorLine id="vat-refusal">Too short.</ErrorLine>);
    expect(screen.getByRole("alert").id).toBe("vat-refusal");
  });

  it("draws a span when it sits inline in its control's row", () => {
    inEnglish(
      <ErrorLine inline id="x">
        Too long.
      </ErrorLine>,
    );
    const line = screen.getByRole("alert");
    expect(line.tagName).toBe("SPAN");
    expect(line.className).toBe("t-danger");
  });

  it("does not announce a standing state, and still wears the ink", () => {
    inEnglish(<ErrorLine standing>Only the owner can post here.</ErrorLine>);
    expect(screen.queryByRole("alert")).toBeNull();
    const line = screen.getByText("Only the owner can post here.");
    expect(line.tagName).toBe("P");
    expect(line.className).toBe("t-danger");
  });
});
