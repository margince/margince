/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { BADGE_TONES, Button } from "./atoms";
import { ProviderMark } from "./provider-mark";
import { PROVIDER_ROW_TONES, ProviderList, ProviderRow } from "./providerrow";

afterEach(cleanup);

describe("ProviderRow", () => {
  it("draws the mark, the name and the status as a toned badge", () => {
    const { container } = render(
      <ProviderRow
        mark={<ProviderMark providerKey="google" />}
        name="Google"
        status={{ tone: "warning", label: "Needs attention" }}
      />,
    );

    expect(container.querySelector(".provider-row-mark svg")).not.toBeNull();
    expect(screen.getByText("Google")).toBeTruthy();
    const badge = screen.getByText("Needs attention").closest(".badge");
    expect(badge?.classList.contains("badge-warning")).toBe(true);
  });

  it("leaves out every slot it was not given, with no empty wrapper", () => {
    const { container } = render(
      <ProviderRow
        mark={<ProviderMark providerKey="google" />}
        name="Google"
      />,
    );

    for (const part of [
      ".badge",
      ".provider-row-account",
      ".provider-row-facts",
      ".provider-row-reason",
      ".provider-row-actions",
      ".provider-row-body",
    ]) {
      expect(container.querySelector(part), part).toBeNull();
    }
  });

  it("puts the one verb before the overflow menu, and children after the row's text", () => {
    const { container } = render(
      <ProviderRow
        mark={<ProviderMark providerKey="microsoft" />}
        name="Microsoft 365"
        account="admin@demo.test"
        facts="Synced 4 minutes ago"
        action={<Button>Reconnect</Button>}
        menu={<Button>More</Button>}
      >
        <p>Importing past email</p>
      </ProviderRow>,
    );

    const actions = container.querySelector(".provider-row-actions");
    expect(actions).not.toBeNull();
    const verbs = within(actions as HTMLElement)
      .getAllByRole("button")
      .map((button) => button.textContent);
    expect(verbs).toEqual(["Reconnect", "More"]);
    expect(screen.getByText("admin@demo.test")).toBeTruthy();
    expect(screen.getByText("Synced 4 minutes ago")).toBeTruthy();

    const text = container.querySelector(".provider-row-text");
    const body = screen.getByText("Importing past email");
    expect(body.closest(".provider-row-body")).not.toBeNull();
    // DOCUMENT_POSITION_FOLLOWING: the body comes after the row's text.
    expect(
      (text as Node).compareDocumentPosition(body) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("makes the name a heading only when asked", () => {
    const { rerender } = render(
      <ProviderRow mark={null} name="Google" headingLevel="h3" />,
    );
    expect(
      screen.getByRole("heading", { level: 3, name: "Google" }),
    ).toBeTruthy();

    rerender(<ProviderRow mark={null} name="Google" />);
    expect(screen.queryByRole("heading")).toBeNull();
  });

  it("draws the reason as one caption line after the facts", () => {
    const { container } = render(
      <ProviderRow
        mark={null}
        name="Microsoft 365"
        status={{ tone: "warning", label: "Needs attention" }}
        facts="Mail"
        reason="Microsoft stopped accepting the sign-in."
        action={<Button variant="primary">Reconnect</Button>}
      />,
    );

    const reason = screen.getByText("Microsoft stopped accepting the sign-in.");
    expect(reason.classList.contains("t-caption")).toBe(true);
    const facts = container.querySelector(".provider-row-facts") as Node;
    expect(
      facts.compareDocumentPosition(reason) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("makes every name in a list an h3 unless the list or the row says otherwise", () => {
    render(
      <>
        <ProviderList>
          <ProviderRow mark={null} name="Google" />
          <ProviderRow mark={null} name="Microsoft 365" headingLevel="h4" />
        </ProviderList>
        <ProviderList headingLevel="h4">
          <ProviderRow mark={null} name="Mail" />
        </ProviderList>
      </>,
    );

    expect(
      screen.getByRole("heading", { level: 3, name: "Google" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 4, name: "Microsoft 365" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { level: 4, name: "Mail" }),
    ).toBeTruthy();
  });

  it("draws rows inside one list", () => {
    const { container } = render(
      <ProviderList>
        <ProviderRow mark={null} name="Google" />
        <ProviderRow mark={null} name="Microsoft 365" />
      </ProviderList>,
    );
    expect(
      container.querySelectorAll(".provider-list > .provider-row"),
    ).toHaveLength(2);
  });
});

describe("PROVIDER_ROW_TONES", () => {
  // A tone Badge does not know falls back to the neutral edge, so a failed
  // connection would read like one nobody has set up.
  it("is a subset of the Badge tones", () => {
    const badgeTones: readonly string[] = BADGE_TONES;
    for (const tone of PROVIDER_ROW_TONES) {
      expect(badgeTones, tone).toContain(tone);
    }
  });
});
