// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../screens/common";
import { Button } from "./atoms";
import { ErrorLine } from "./errorline";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

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

  it("draws nothing for a guard's false, so `isError && error` goes in straight", () => {
    const { container } = inEnglish(<ErrorLine error={false} />);
    expect(container.innerHTML).toBe("");
  });

  it("draws nothing for an empty string, as an error or as the sentence", () => {
    const { container } = inEnglish(
      <>
        <ErrorLine error="" />
        <ErrorLine>{""}</ErrorLine>
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

describe("ErrorLine in a dialog", () => {
  const watchScrolls = () =>
    vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => {});

  it("scrolls itself into view when it arrives inside a dialog", () => {
    const scroll = watchScrolls();
    inEnglish(
      <div className="modal">
        <ErrorLine>The save was refused.</ErrorLine>
      </div>,
    );
    expect(scroll).toHaveBeenCalledTimes(1);
    expect(scroll.mock.calls[0]?.[0]).toMatchObject({ block: "nearest" });
  });

  it("scrolls again when the message changes, and not on a re-render of the same one", () => {
    const scroll = watchScrolls();
    const view = (message: string) => (
      <LocaleProvider initial="en">
        <div className="modal">
          <ErrorLine>{message}</ErrorLine>
        </div>
      </LocaleProvider>
    );
    const { rerender } = render(view("First refusal."));
    rerender(view("First refusal."));
    expect(scroll).toHaveBeenCalledTimes(1);
    rerender(view("Second refusal."));
    expect(scroll).toHaveBeenCalledTimes(2);
  });

  it("scrolls for each failed attempt, even when the refusal reads the same", () => {
    const scroll = watchScrolls();
    const view = (error: Error) => (
      <LocaleProvider initial="en">
        <div className="modal">
          <ErrorLine error={error} />
        </div>
      </LocaleProvider>
    );
    const first = new Error("The save was refused.");
    const { rerender } = render(view(first));
    rerender(view(first));
    expect(scroll).toHaveBeenCalledTimes(1);
    rerender(view(new Error("The save was refused.")));
    expect(scroll).toHaveBeenCalledTimes(2);
  });

  it("leaves the page where it is outside a dialog, and for a standing state", () => {
    const scroll = watchScrolls();
    inEnglish(
      <>
        <ErrorLine>The save was refused.</ErrorLine>
        <div className="modal">
          <ErrorLine standing>Only the owner can post here.</ErrorLine>
        </div>
      </>,
    );
    expect(scroll).not.toHaveBeenCalled();
  });
});
