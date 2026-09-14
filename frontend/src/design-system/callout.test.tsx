// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MailX } from "lucide-react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Callout, type CalloutKind, type CalloutTone } from "./callout";
import { FactList } from "./factlist";

afterEach(cleanup);

/**
 * The tone's glyph, named by the class lucide draws it under. `accent` shares
 * `info`'s on purpose — it is the same claim said emphatically — which is why
 * the pairing gets a test of its own below rather than resting on this table.
 */
const TONE_GLYPHS: ReadonlyArray<readonly [CalloutTone, string]> = [
  ["info", "lucide-info"],
  ["accent", "lucide-info"],
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
  ["outcome", "accent", "status"],
  ["outcome", "warn", "status"],
  ["outcome", "danger", "alert"],
  ["outcome", "success", "status"],
  ["event", "info", "status"],
  ["event", "accent", "status"],
  ["event", "warn", "status"],
  ["event", "danger", "status"],
  ["event", "success", "status"],
  ["standing", "info", null],
  ["standing", "accent", null],
  ["standing", "warn", null],
  ["standing", "danger", null],
  ["standing", "success", null],
  [undefined, "info", null],
  [undefined, "accent", null],
  [undefined, "warn", null],
  [undefined, "danger", null],
  [undefined, "success", null],
];

describe("Callout", () => {
  it("stays silent unless the caller says it appeared for a reason", () => {
    const { container } = render(<Callout title="Nothing urgent" />);
    // A notice rendered with the page has nothing to interrupt for. Announcing
    // every one of them is how a reader learns to ignore the ones that matter.
    expect(container.querySelector("[role]")).toBeNull();
  });

  it("interrupts only where the caller asked for it", () => {
    render(
      <Callout tone="danger" live="alert" title="That did not save">
        The role changed while you were editing.
      </Callout>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("That did not save");
  });

  it.each(TONES)(
    "carries the %s tone as a class rather than a colour",
    (tone) => {
      // Tone is never the only signal — the words carry the meaning — but it has
      // to reach CSS as something the theme can restyle in both palettes, and
      // every tone has to reach it, not only the one somebody tested.
      const { container } = render(<Callout tone={tone} title="Running low" />);
      expect(container.querySelector(`.callout-${tone}`)).toBeInTheDocument();
    },
  );

  it("is complete with a heading alone, and adds a body only when given one", () => {
    // One anatomy, and the heading is all of it that is mandatory: most notices
    // are a single sentence, and that sentence is the heading. An empty body
    // box under it would put the heading's own gap below the last word.
    const { container, rerender } = render(<Callout title="Reindex needed" />);
    expect(screen.getByText("Reindex needed")).toBeInTheDocument();
    expect(container.querySelector(".callout-text")).toBeNull();

    rerender(<Callout title="Reindex needed">The index is behind.</Callout>);
    expect(container.querySelector(".callout-text")).toHaveTextContent(
      "The index is behind.",
    );
  });

  it("puts the caller's verbs in the actions slot and the dismiss in its own", () => {
    // Two slots at the end of the card, in one order: what the reader can do
    // about the notice, then putting it away. A verb that landed in the dismiss
    // slot — or in the body, which is where the call sites had been putting it
    // — reads as neither.
    const { container } = render(
      <Callout
        title="Reindex needed"
        actions={<button type="button">Open</button>}
        dismiss={{ label: "Dismiss", onDismiss: () => {} }}
      />,
    );
    expect(
      container.querySelector(".callout-actions button"),
    ).toHaveAccessibleName("Open");
    expect(
      container.querySelector(".callout-dismiss button"),
    ).toHaveAccessibleName("Dismiss");
  });

  it("draws no actions slot when the notice has nothing to do about it", () => {
    const { container } = render(<Callout title="Capture is running" />);
    expect(container.querySelector(".callout-actions")).toBeNull();
  });

  it.each(TONE_GLYPHS)("draws the %s tone's own glyph", (tone, glyph) => {
    // Dots differing only in hue are one signal wearing several coats. A shape
    // per tone is what reaches a reader who does not read colour.
    const { container } = render(<Callout tone={tone} title="Something" />);
    expect(container.querySelector(`.callout-icon .${glyph}`)).toBeTruthy();
  });

  it("gives the accent tone info's glyph rather than a second one", () => {
    // `accent` is `info` asking for more attention, not a different kind of
    // claim, so the shape must not say the two are about different things — and
    // the glyph is READ off `info`'s own render rather than named again here.
    const informational = render(<Callout title="An account is waiting" />);
    const glyph = informational.container
      .querySelector(".callout-icon svg")
      ?.getAttribute("class");
    cleanup();

    const emphatic = render(
      <Callout tone="accent" title="An account is waiting" />,
    );
    expect(
      emphatic.container.querySelector(".callout-icon svg"),
    ).toHaveAttribute("class", glyph);
  });

  it("takes the caller's glyph instead where the notice names a thing", () => {
    const { container } = render(
      <Callout tone="warn" icon={MailX} title="Nobody has written back" />,
    );
    expect(
      container.querySelector(".callout-icon .lucide-mail-x"),
    ).toBeTruthy();
    expect(container.querySelector(".lucide-triangle-alert")).toBeNull();
  });

  it("states the announcement for every kind against every tone", () => {
    // A table that quietly covers nine of twenty pairs reports PASS for the
    // eleven it never rendered, and there is no failing assertion to notice.
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
        <Callout kind={kind} tone={tone} title="Something happened" />,
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
      <Callout
        kind="standing"
        live="alert"
        title="The licence expires tonight"
      />,
    );
    expect(container.querySelector(".callout")).toHaveAttribute(
      "role",
      "alert",
    );

    rerender(
      <Callout
        kind="outcome"
        tone="danger"
        live="status"
        title="One of eleven rows was refused"
      />,
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
      <Callout
        tone="success"
        title="HubSpot is connected"
        dismiss={{ label: "Dismiss", onDismiss }}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("offers no dismiss control unless the caller handles it", () => {
    const { container } = render(
      <Callout tone="success" title="HubSpot is connected" />,
    );
    expect(container.querySelector(".callout-dismiss")).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
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
