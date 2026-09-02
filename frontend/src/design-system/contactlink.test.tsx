/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ContactLink } from "./contactlink";

afterEach(cleanup);

describe("ContactLink", () => {
  it("links a mailbox with mailto and a number with tel", () => {
    render(
      <>
        <ContactLink kind="email" value="dana@brandt.example" />
        <ContactLink kind="phone" value="+33 6 12 44 08 91" />
      </>,
    );
    expect(
      screen
        .getByRole("link", { name: "dana@brandt.example" })
        .getAttribute("href"),
    ).toBe("mailto:dana@brandt.example");
    expect(
      screen
        .getByRole("link", { name: "+33 6 12 44 08 91" })
        .getAttribute("href"),
    ).toBe("tel:+33612440891");
  });

  it("keeps a refused value as text rather than hiding it", () => {
    render(<ContactLink kind="email" value="dana@brandt.example?bcc=x" />);
    expect(screen.queryByRole("link")).toBeNull();
    const text = screen.getByText("dana@brandt.example?bcc=x");
    // Text, and dressed as text: the link's affordance would promise a click
    // that does nothing.
    expect(text.className).toBe("");
  });

  it("dresses a refused value only in the class the caller gives the text", () => {
    render(
      <ContactLink
        kind="email"
        value="dana@brandt.example?bcc=x"
        className="link-button t-mono"
        textClassName="t-mono"
      />,
    );
    expect(screen.getByText("dana@brandt.example?bcc=x").className).toBe(
      "t-mono",
    );
  });

  it("lets the caller lead the value with a glyph", () => {
    render(
      <ContactLink kind="email" value="dana@brandt.example">
        <span aria-hidden="true">✉</span> dana@brandt.example
      </ContactLink>,
    );
    expect(screen.getByRole("link").textContent).toContain(
      "dana@brandt.example",
    );
  });
});
