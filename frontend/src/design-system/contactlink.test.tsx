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
    expect(screen.getByText("dana@brandt.example?bcc=x")).toBeTruthy();
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
