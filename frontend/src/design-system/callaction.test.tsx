// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { telUri } from "../format/contacturi";
import { CallAction } from "./callaction";

afterEach(cleanup);

const DIALS = "+49 30 1234 5678";
const REFUSED = "ask reception";
const NO_PHONE = "No phone number on this contact.";

function describedTextOf(control: HTMLElement): string {
  return (control.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();
}

describe("CallAction dials through telUri", () => {
  it("draws the number as a link to its tel:", () => {
    render(<CallAction appearance="value" number={DIALS} />);
    const link = screen.getByRole("link", { name: DIALS });
    expect(link.getAttribute("href")).toBe(telUri(DIALS));
    expect(link.getAttribute("target")).toBeNull();
  });

  it("draws the labelled verb as a link to its tel:", () => {
    render(<CallAction appearance="button" number={DIALS} label="Call" />);
    expect(
      screen.getByRole("link", { name: "Call" }).getAttribute("href"),
    ).toBe(telUri(DIALS));
  });

  it("draws the square glyph as a link to its tel:", () => {
    render(
      <CallAction
        appearance="icon"
        number={DIALS}
        label="Call"
        reason={NO_PHONE}
      />,
    );
    expect(
      screen.getByRole("link", { name: "Call" }).getAttribute("href"),
    ).toBe(telUri(DIALS));
  });
});

describe("CallAction with nothing to dial", () => {
  it("keeps a refused number as plain text", () => {
    render(<CallAction appearance="value" number={REFUSED} />);
    expect(screen.getByText(REFUSED).tagName).toBe("SPAN");
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("draws no verb for a refused number or no number", () => {
    const { container } = render(
      <>
        <CallAction appearance="button" number={REFUSED} label="Call" />
        <CallAction appearance="button" number={undefined} label="Call" />
      </>,
    );
    expect(container.childElementCount).toBe(0);
  });

  it("refuses the glyph with the caller's reason when there is no number", () => {
    render(
      <CallAction
        appearance="icon"
        number={undefined}
        label="Call"
        reason={NO_PHONE}
      />,
    );
    const control = screen.getByRole("button", { name: "Call" });
    expect(control.hasAttribute("disabled")).toBe(true);
    expect(describedTextOf(control)).toBe(NO_PHONE);
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("refuses the glyph for a number telUri refuses", () => {
    render(
      <CallAction
        appearance="icon"
        number={REFUSED}
        label="Call"
        reason={NO_PHONE}
      />,
    );
    expect(describedTextOf(screen.getByRole("button", { name: "Call" }))).toBe(
      NO_PHONE,
    );
  });

  it("lets the page's refusal win over a number on file", () => {
    render(
      <>
        <CallAction
          appearance="icon"
          number={DIALS}
          label="Call"
          reason={NO_PHONE}
          reasonId="archived"
        />
        <p id="archived">This contact is archived.</p>
      </>,
    );
    const control = screen.getByRole("button", { name: "Call" });
    expect(control.hasAttribute("disabled")).toBe(true);
    expect(describedTextOf(control)).toBe("This contact is archived.");
    expect(screen.queryByText(NO_PHONE)).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
