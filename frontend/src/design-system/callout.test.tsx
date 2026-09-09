// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MailX } from "lucide-react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Callout, type CalloutKind, type CalloutTone } from "./callout";
import { FactList } from "./factlist";

afterEach(cleanup);

/** The tone's glyph, named by the class lucide draws it under. */
const TONE_GLYPHS: ReadonlyArray<readonly [CalloutTone, string]> = [
  ["info", "lucide-info"],
  ["warn", "lucide-triangle-alert"],
  ["danger", "lucide-circle-x"],
  ["success", "lucide-circle-check"],
];

/** Every kind a caller can pass, passing none included. */
const KINDS: ReadonlyArray<CalloutKind | undefined> = [
  "outcome",
  "event",
  "standing",
  undefined,
];

/** The tones, read off the glyph table rather than listed a second time. */
const TONES: ReadonlyArray<CalloutTone> = TONE_GLYPHS.map(([tone]) => tone);

/**
 * The whole derivation as a table: what the notice IS, how bad the news is, and
 * how loudly a screen reader is told. `null` is the silent case — a notice
 * rendered with the page has nothing to interrupt for.
 *
 * Every pair is stated rather than only the interesting ones, and the census
 * below fails when one goes missing: the pairs that look uninteresting today
 * are exactly where a change to the derivation would land unnoticed.
 */
const DERIVATIONS: ReadonlyArray<
  readonly [CalloutKind | undefined, CalloutTone, string | null]
> = [
  ["outcome", "info", "status"],
  ["outcome", "warn", "status"],
  ["outcome", "danger", "alert"],
  ["outcome", "success", "status"],
  ["event", "info", "status"],
  ["event", "warn", "status"],
  ["event", "danger", "status"],
  ["event", "success", "status"],
  ["standing", "info", null],
  ["standing", "warn", null],
  ["standing", "danger", null],
  ["standing", "success", null],
  [undefined, "info", null],
  [undefined, "warn", null],
  [undefined, "danger", null],
  [undefined, "success", null],
];

describe("Callout", () => {
  it("stays silent unless the caller says it appeared for a reason", () => {
    const { container } = render(<Callout>Nothing urgent.</Callout>);
    // A notice rendered with the page has nothing to interrupt for. Announcing
    // every one of them is how a reader learns to ignore the ones that matter.
    expect(container.querySelector("[role]")).toBeNull();
  });

  it("interrupts only where the caller asked for it", () => {
    render(
      <Callout tone="danger" live="alert">
        That did not save.
      </Callout>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("That did not save.");
  });

  it("carries the tone as a class rather than a colour", () => {
    const { container } = render(<Callout tone="warn">Running low.</Callout>);
    // Tone is never the only signal — the words carry the meaning — but it has
    // to reach CSS as something the theme can restyle in both palettes.
    expect(container.querySelector(".callout-warn")).toBeInTheDocument();
  });

  it("renders a title and actions when given them, and neither when not", () => {
    const { container, rerender } = render(
      <Callout
        title="Reindex needed"
        actions={<button type="button">Open</button>}
      >
        The index is behind.
      </Callout>,
    );
    expect(screen.getByText("Reindex needed")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open" })).toBeInTheDocument();

    rerender(<Callout>The index is behind.</Callout>);
    expect(container.querySelector(".callout-title")).toBeNull();
    expect(container.querySelector(".callout-actions")).toBeNull();
  });

  it.each(TONE_GLYPHS)("draws the %s tone's own glyph", (tone, glyph) => {
    // Four dots differing only in hue are one signal wearing four coats. A
    // shape per tone is what reaches a reader who does not read colour.
    const { container } = render(<Callout tone={tone}>Something.</Callout>);
    expect(container.querySelector(`.callout-icon .${glyph}`)).toBeTruthy();
  });

  it("takes the caller's glyph instead where the notice names a thing", () => {
    const { container } = render(
      <Callout tone="warn" icon={MailX}>
        Nobody has written back.
      </Callout>,
    );
    expect(
      container.querySelector(".callout-icon .lucide-mail-x"),
    ).toBeTruthy();
    expect(container.querySelector(".lucide-triangle-alert")).toBeNull();
  });

  it("states the announcement for every kind against every tone", () => {
    // A table that quietly covers nine of sixteen pairs reports PASS for the
    // seven it never rendered, and there is no failing assertion to notice.
    const stated = DERIVATIONS.map(([kind, tone]) => `${kind}/${tone}`).sort();
    const pairs = KINDS.flatMap((kind) =>
      TONES.map((tone) => `${kind}/${tone}`),
    ).sort();
    expect(stated).toEqual(pairs);
  });

  it.each(DERIVATIONS)(
    "announces a %s notice in %s as %s",
    (kind, tone, role) => {
      const { container } = render(
        <Callout kind={kind} tone={tone}>
          Something happened.
        </Callout>,
      );
      // `getAttribute` rather than `toHaveAttribute`, because the silent case
      // is the ABSENCE of the attribute and this reads both cases the same way.
      expect(container.querySelector(".callout")?.getAttribute("role")).toBe(
        role,
      );
    },
  );

  it("lets an explicit loudness win over the derived one", () => {
    // The caller knows something the kind does not: a standing notice this
    // screen has decided must interrupt, and a failure it has decided must not.
    const { container, rerender } = render(
      <Callout kind="standing" live="alert">
        The licence expires tonight.
      </Callout>,
    );
    expect(container.querySelector(".callout")).toHaveAttribute(
      "role",
      "alert",
    );

    rerender(
      <Callout kind="outcome" tone="danger" live="status">
        One of eleven rows was refused.
      </Callout>,
    );
    expect(container.querySelector(".callout")).toHaveAttribute(
      "role",
      "status",
    );
  });

  it("puts the notice away from a control that has a name", async () => {
    const onDismiss = vi.fn();
    const user = userEvent.setup();
    render(
      <Callout tone="success" dismiss={{ label: "Dismiss", onDismiss }}>
        HubSpot is connected.
      </Callout>,
    );
    await user.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("offers no dismiss control unless the caller handles it", () => {
    const { container } = render(
      <Callout tone="success">HubSpot is connected.</Callout>,
    );
    expect(container.querySelector(".callout-dismiss")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("wears the face its heading asks for", () => {
    // The two faces are two densities, and CSS picks between them by class: a
    // titled card, or the single row a dialog footer can afford.
    const { container, rerender } = render(
      <Callout title="Reindex needed">The index is behind.</Callout>,
    );
    expect(container.querySelector(".callout-titled")).toBeTruthy();
    expect(container.querySelector(".callout-compact")).toBeNull();

    rerender(<Callout>The index is behind.</Callout>);
    expect(container.querySelector(".callout-compact")).toBeTruthy();
    expect(container.querySelector(".callout-titled")).toBeNull();
  });
});

describe("FactList", () => {
  it("pairs every term with its value", () => {
    render(
      <FactList
        facts={[
          { key: "in", term: "Last inbound", value: "3 Feb 2026" },
          { key: "out", term: "Last outbound", value: "Never" },
        ]}
      />,
    );
    const terms = screen.getAllByRole("term").map((node) => node.textContent);
    const values = screen
      .getAllByRole("definition")
      .map((node) => node.textContent);
    expect(terms).toEqual(["Last inbound", "Last outbound"]);
    expect(values).toEqual(["3 Feb 2026", "Never"]);
  });

  it("keeps rows apart when the term repeats, which the real data does", () => {
    render(
      <FactList
        facts={[
          { key: "email-1", term: "Email", value: "a@acme.test" },
          { key: "email-2", term: "Email", value: "b@acme.test" },
        ]}
      />,
    );
    expect(screen.getAllByRole("term")).toHaveLength(2);
    expect(screen.getByText("b@acme.test")).toBeInTheDocument();
  });

  it("puts a note under its value rather than beside it", () => {
    const { container } = render(
      <FactList
        facts={[
          {
            key: "spend",
            term: "Spend",
            value: "€1,200",
            note: "Partial month",
          },
        ]}
      />,
    );
    const note = container.querySelector(".factlist-note");
    expect(note).toHaveTextContent("Partial month");
    // Inside the value, so a reader hears the qualifier with the figure it
    // qualifies rather than as a row of its own.
    expect(note?.closest(".factlist-value")).not.toBeNull();
  });
});
