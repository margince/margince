// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties, ReactNode } from "react";
import { expect } from "storybook/test";
import { Panel, PanelBody } from "./panel";
import {
  specimenColumn,
  specimenIntro,
  specimenNote,
  specimenToken,
  useTokenValue,
} from "./tokenspecimen";

/**
 * Depth is light first and shadow second. A surface stands off the one under
 * it by its own lightness, so the resting shadow can stay slight. Flip the
 * theme in the toolbar: on a dark ground every level lifts toward the light.
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

function Label({ token, usage }: Readonly<{ token: string; usage: string }>) {
  return (
    <div style={label}>
      <code className={specimenToken}>{token}</code>
      <span className={specimenNote}>{usage}</span>
    </div>
  );
}

const ground: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-4)",
  padding: "var(--space-6)",
  background: "var(--bgPage)",
  border: "1px solid var(--borderSubtle)",
  borderRadius: "var(--r-lg)",
};
const swatches: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(120px, 1fr))",
  gap: "var(--space-3)",
};

// The rung each level paints, drawn straight from its tokens: the play below
// holds every real pane and its head against these.
const RUNGS = [
  {
    probe: "1",
    token: "--ladder1",
    edge: "--ladder1Edge",
    usage: "Level 1: a pane on the page.",
  },
  {
    probe: "2",
    token: "--ladder2",
    edge: "--ladder2Edge",
    usage: "Level 2: a pane in a pane.",
  },
  { probe: "3", token: "--ladder3", edge: "--ladder3Edge", usage: "Level 3." },
  {
    probe: "4",
    token: "--ladder4",
    edge: "--ladder4Edge",
    usage: "Level 4 and deeper.",
  },
  {
    probe: "overlay",
    token: "--bgElevated",
    edge: "--ladder1Edge",
    usage: "An overlay's own surface.",
  },
] as const;

function Level({ level, children }: { level: string; children?: ReactNode }) {
  return (
    <Panel title={level}>
      <PanelBody>{children}</PanelBody>
    </Panel>
  );
}

/**
 * The ladder as the product draws it: real panels nested on the page. The same
 * panels sit inside an overlay, which counts again from its own surface.
 */
export const Surfaces: Story = {
  render: () => (
    <div className={specimenColumn} style={wide}>
      <p className={specimenIntro}>
        A pane takes its ground from its depth, not its kind. A pane on the page
        is level 1, a pane inside it level 2, then level 3, then level 4 for
        anything deeper. An overlay keeps its own surface and counts again: a
        card in a dialog is level 2. Nest a Panel and it takes the right ground.
      </p>
      <div style={swatches}>
        {RUNGS.map((rung) => (
          <div
            key={rung.probe}
            data-probe={rung.probe}
            style={{
              ...box,
              background: `var(${rung.token})`,
              borderColor: `var(${rung.edge})`,
            }}
          >
            <Label token={rung.token} usage={rung.usage} />
          </div>
        ))}
      </div>
      <div style={ground} data-ladder="page">
        <Level level="Level 1">
          <Level level="Level 2">
            <Level level="Level 3">
              <Level level="Level 4">
                <Level level="Level 5 keeps level 4" />
              </Level>
            </Level>
          </Level>
        </Level>
      </div>
      <div style={ground}>
        <div className="modal" data-ladder="overlay">
          <Level level="Level 2, in an overlay">
            <Level level="Level 3" />
          </Level>
        </div>
      </div>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const paint = (element: Element | null) =>
      element === null ? "" : getComputedStyle(element).backgroundColor;
    const edge = (element: Element | null) =>
      element === null ? "" : getComputedStyle(element).borderTopColor;
    const seam = (element: Element | null) =>
      element === null ? "" : getComputedStyle(element).borderBottomColor;
    const probe = (name: string) =>
      canvasElement.querySelector(`[data-probe="${name}"]`);
    const panes = (ladder: string) => [
      ...canvasElement.querySelectorAll(
        `[data-ladder="${ladder}"] section.panel`,
      ),
    ];
    const heads = (ladder: string) =>
      panes(ladder).map((pane) => pane.querySelector(":scope > .panel-head"));
    const page = ["1", "2", "3", "4", "4"].map(probe);
    await expect(panes("page").map(paint)).toEqual(page.map(paint));
    await expect(panes("page").map(edge)).toEqual(page.map(edge));
    await expect(heads("page").map(seam)).toEqual(page.map(edge));
    const overlay = canvasElement.querySelector('[data-ladder="overlay"]');
    await expect(paint(overlay)).toBe(paint(probe("overlay")));
    const inOverlay = ["2", "3"].map(probe);
    await expect(panes("overlay").map(paint)).toEqual(inOverlay.map(paint));
    await expect(panes("overlay").map(edge)).toEqual(inOverlay.map(edge));
    await expect(heads("overlay").map(seam)).toEqual(inOverlay.map(edge));
  },
};

/**
 * A pane is one zone over the page ground, edged with a hairline; a pane in
 * it lies one level down.
 */
export const Pane: Story = {
  render: () => (
    <div className={specimenColumn} style={{ maxWidth: "880px" }}>
      <p className={specimenIntro}>
        Use a pane for one zone over the page ground. A pane inside it takes the
        next level's ground and hairline by itself.
      </p>
      <div style={ground}>
        <Level level="A zone on the page">
          <Level level="A card in the zone" />
        </Level>
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
