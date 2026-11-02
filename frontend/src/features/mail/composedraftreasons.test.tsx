/** @vitest-environment happy-dom */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { DraftReasons } from "./composedraftcontext";

// What a draft stands on is one press away: the band shows the hint, and the
// inputs open under it only when the rep asks.

const REASONS = [
  { kind: "intent", label: "Ask for the signed order" },
  { kind: "deal", label: "Renewal closes this quarter" },
] as const;

function renderReasons() {
  render(
    <LocaleProvider>
      <DraftReasons reasons={REASONS} />
    </LocaleProvider>,
  );
  const hint = screen.getByText("Why this draft").closest("summary");
  if (hint === null) {
    throw new Error("the hint is not a disclosure summary");
  }
  return hint;
}

describe("DraftReasons", () => {
  it("starts shut, naming only the hint", () => {
    const hint = renderReasons();
    expect(hint.getAttribute("aria-expanded")).toBe("false");
    expect(hint.closest("details")?.open).toBe(false);
  });

  it("opens to the based-on line and one chip per input", async () => {
    const hint = renderReasons();
    await userEvent.click(hint);
    expect(hint.getAttribute("aria-expanded")).toBe("true");
    expect(
      screen.getByText(
        "Based on: Ask for the signed order · Renewal closes this quarter",
      ),
    ).toBeTruthy();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });

  it("renders nothing when the draft names no inputs", () => {
    const { container } = render(
      <LocaleProvider>
        <DraftReasons reasons={[]} />
      </LocaleProvider>,
    );
    expect(container.textContent).toBe("");
  });
});
