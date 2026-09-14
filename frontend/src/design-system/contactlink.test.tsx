/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WriteToProvider } from "../screens/writeto";
import { ContactLink } from "./contactlink";

afterEach(cleanup);

const dana = { entityType: "contact", entityId: "p-1" } as const;

describe("ContactLink", () => {
  it("opens the composer on the address's record, and links a number with tel", () => {
    const writeTo = vi.fn();
    render(
      <WriteToProvider writeTo={writeTo}>
        <ContactLink kind="email" value=" dana@brandt.example " record={dana} />
        <ContactLink kind="phone" value="+33 6 12 44 08 91" />
      </WriteToProvider>,
    );
    // A button and not a link: nothing here has an href for the reader's own
    // mail client to take.
    fireEvent.click(
      screen.getByRole("button", { name: "dana@brandt.example" }),
    );
    expect(writeTo).toHaveBeenCalledWith({
      entityType: "contact",
      entityId: "p-1",
      address: "dana@brandt.example",
    });
    expect(
      screen
        .getByRole("link", { name: "+33 6 12 44 08 91" })
        .getAttribute("href"),
    ).toBe("tel:+33612440891");
  });

  it("keeps a refused value as text rather than hiding it", () => {
    render(
      <WriteToProvider writeTo={vi.fn()}>
        <ContactLink
          kind="email"
          value="dana@brandt.example?bcc=x"
          record={dana}
        />
      </WriteToProvider>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    const text = screen.getByText("dana@brandt.example?bcc=x");
    // Text, and dressed as text: the control's affordance would promise a
    // press that does nothing.
    expect(text.className).toBe("");
  });

  it("keeps the address as text on a record that takes no writes", () => {
    render(
      <WriteToProvider writeTo={vi.fn()}>
        <ContactLink
          kind="email"
          value="dana@brandt.example"
          record={dana}
          readOnly
        />
      </WriteToProvider>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("dana@brandt.example")).toBeTruthy();
  });

  it("hands the address to the reader's own client where nothing writes from the product", () => {
    // No host, or a reader with no connected mailbox: the same case, and the
    // `mailto:` is the honest answer to it rather than a press that opens
    // nothing.
    render(
      <ContactLink kind="email" value="dana@brandt.example" record={dana} />,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(
      screen
        .getByRole("link", { name: "dana@brandt.example" })
        .getAttribute("href"),
    ).toBe("mailto:dana@brandt.example");
  });

  it("dresses a refused value only in the class the caller gives the text", () => {
    render(
      <ContactLink
        kind="email"
        value="dana@brandt.example?bcc=x"
        record={dana}
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
      <WriteToProvider writeTo={vi.fn()}>
        <ContactLink kind="email" value="dana@brandt.example" record={dana}>
          <span aria-hidden="true">✉</span> dana@brandt.example
        </ContactLink>
      </WriteToProvider>,
    );
    expect(screen.getByRole("button").textContent).toContain(
      "dana@brandt.example",
    );
  });
});
