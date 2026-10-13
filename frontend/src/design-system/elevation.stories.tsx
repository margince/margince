// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties, ReactNode } from "react";
import {
  specimenColumn,
  specimenIntro,
  specimenNote,
  specimenToken,
  useTokenValue,
} from "./tokenspecimen";

/**
 * Depth is light first and shadow second. A surface is separated from the one
 * under it by its own lightness, which is what lets the resting shadow be as
 * slight as it is. The shadow then depends on what the thing is: a surface and
 * a filled control have a top side, a field has a floor, and what is truly
 * above the plane (a popover, a menu, a drawer) takes the pop.
 *
 * Flip the theme in the toolbar. On a dark ground every surface lifts toward
 * the light, so the ladder runs the other way and the shadows deepen.
 */
const meta = {
  title: "Foundations/Elevation",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const box: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-3)",
  padding: "var(--space-4)",
  border: "1px solid var(--borderSubtle)",
  borderRadius: "var(--r-md)",
  color: "var(--textPrimary)",
};
const label: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-1)",
};
const wide: CSSProperties = { maxWidth: "720px" };

type Rung = Readonly<{ token: string; usage: string }>;

function Label({ token, usage }: Rung) {
  return (
    <div style={label}>
      <code className={specimenToken}>{token}</code>
      <span className={specimenNote}>{usage}</span>
    </div>
  );
}

function Nest({ rung, children }: { rung: Rung; children?: ReactNode }) {
  return (
    <div style={{ ...box, background: `var(${rung.token})` }}>
      <Label {...rung} />
      {children}
    </div>
  );
}

/**
 * The ladder, drawn as it is stacked on a record page: the page, a well cut
 * into it, a card on that well, and the elevated surface a card or a panel
 * is made of. Each rung is one visible step off its neighbours, and a card is
 * separated by its own lightness rather than by the shadow alone.
 */
export const Surfaces: Story = {
  render: () => (
    <div className={specimenColumn} style={wide}>
      <p className={specimenIntro}>
        Elevation here is light first and shadow second. A surface is separated
        from the one under it by its own lightness, so choose the rung by what
        the thing is: the page to read on, a well for a recess, a card for an
        inset plate, the elevated surface for what lifts off the page. Do not
        put a pane inside a pane.
      </p>
      <Nest rung={{ token: "--bgPage", usage: "The reading ground." }}>
        <Nest
          rung={{
            token: "--bgWell",
            usage:
              "A recess in an elevated surface, so the panes inside it are the lifted thing.",
          }}
        >
          <Nest
            rung={{
              token: "--bgInset",
              usage: "A well inside a card: an inset row, a plate.",
            }}
          >
            <Nest
              rung={{
                token: "--bgElevated",
                usage:
                  "A card, a panel, a menu. The brightest thing on screen in light.",
              }}
            />
          </Nest>
        </Nest>
      </Nest>
    </div>
  ),
};

/**
 * A pane is one translucent zone over the lit ground, edged with a hairline.
 */
export const Pane: Story = {
  render: () => (
    <div className={specimenColumn} style={{ maxWidth: "880px" }}>
      <p className={specimenIntro}>
        Use a pane for one zone over the lit ground: a translucent fill with a
        blur behind it, and a hairline for its edge.
      </p>
      <div
        style={{
          padding: "var(--space-4)",
          background: "var(--groundLit)",
          border: "1px solid var(--borderSubtle)",
          borderRadius: "var(--r-lg)",
        }}
      >
        <div
          style={{
            ...box,
            background: "var(--pane)",
            border: "1px solid var(--paneEdge)",
          }}
        >
          <Label
            token="--pane"
            usage="One translucent pane per zone, with --paneEdge for its hairline."
          />
        </div>
      </div>
    </div>
  ),
};

const stage: CSSProperties = {
  position: "relative",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: "260px",
  padding: "var(--space-8)",
  background: "var(--bgPage)",
  border: "1px solid var(--borderSubtle)",
  borderRadius: "var(--r-lg)",
  overflow: "hidden",
};
const scrim: CSSProperties = {
  position: "absolute",
  inset: 0,
  background: "var(--overlayScrim)",
};
const floating: CSSProperties = {
  ...box,
  position: "relative",
  background: "var(--bgElevated)",
  boxShadow: "var(--shadow-pop)",
  maxWidth: "320px",
};

/**
 * What sits above the plane: a scrim over the page, and a surface that
 * takes the pop shadow over it. Nothing at rest wears the pop.
 */
export const OverlaysAndScrim: Story = {
  name: "Overlays and scrim",
  render: () => (
    <div className={specimenColumn} style={wide}>
      <p className={specimenIntro}>
        What is genuinely above the plane takes a scrim behind it and the pop
        shadow on it. Use this for a modal, a popover, a menu or a drawer, and
        for nothing at rest.
      </p>
      <div style={stage}>
        <div style={scrim} />
        <div style={floating}>
          <Label
            token="--bgElevated"
            usage="A modal, popover or drawer is an elevated surface under --shadow-pop."
          />
          <Label
            token="--overlayScrim"
            usage="The backdrop that dims the page behind it."
          />
        </div>
      </div>
    </div>
  ),
};

const SHADOWS: ReadonlyArray<
  Readonly<{ token: string; role: string; when: string; style: CSSProperties }>
> = [
  {
    token: "--shadow-rest",
    role: "A surface at rest",
    when: "A pane, a card, a reading, a filled control. One tight layer that gives a flat rectangle a top side. A control drops it on hover, active and focus: pressed into the page, not lifted off it.",
    style: { boxShadow: "var(--shadow-rest)" },
  },
  {
    token: "--shadow-well",
    role: "A field",
    when: "A text box, a textarea, a field shell. The same layer turned inside: a field is a place to put something, so it has a floor.",
    style: {
      boxShadow: "var(--shadow-well)",
      border: "1px solid var(--borderControl)",
    },
  },
  {
    token: "--shadow-pop",
    role: "Above the plane",
    when: "A popover, a menu, a drawer. Nothing at rest wears it, and nothing wears both it and the resting layer.",
    style: { boxShadow: "var(--shadow-pop)" },
  },
];

function ShadowCard({ shadow }: { shadow: (typeof SHADOWS)[number] }) {
  const { ref, value } = useTokenValue<HTMLDivElement>(shadow.token);
  return (
    <div className={specimenColumn}>
      <div
        ref={ref}
        style={{
          ...box,
          background: "var(--bgElevated)",
          minHeight: "var(--space-16)",
          ...shadow.style,
        }}
      >
        <strong>{shadow.role}</strong>
      </div>
      <code className={specimenToken}>{shadow.token}</code>
      <code className={specimenNote}>{value}</code>
      <span className={specimenNote}>{shadow.when}</span>
    </div>
  );
}

/**
 * Three shadows, and no stacking of a shadow under a shadow. Each specimen is
 * an elevated surface on the page ground, and its value is read from the
 * theme in force.
 */
export const Shadows: Story = {
  render: () => (
    <div className={specimenColumn} style={{ maxWidth: "960px" }}>
      <p className={specimenIntro}>
        Three shadows, chosen by what the thing is. A surface or filled control
        at rest takes the resting layer, a field takes the same layer turned
        inside, and what floats above the plane takes the pop. Never stack a
        shadow under a shadow.
      </p>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(3, minmax(220px, 1fr))",
          gap: "var(--space-6)",
          padding: "var(--space-8)",
          background: "var(--bgPage)",
          borderRadius: "var(--r-lg)",
        }}
      >
        {SHADOWS.map((shadow) => (
          <ShadowCard key={shadow.token} shadow={shadow} />
        ))}
      </div>
    </div>
  ),
};
