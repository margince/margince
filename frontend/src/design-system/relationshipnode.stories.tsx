// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef } from "react";
import { expect } from "storybook/test";
import type { RelationshipMapLabels } from "./relationshipmap";
import "./relationshipmap.css";
import {
  COL_W,
  type MapNode,
  NODE_H,
  type Placed,
} from "./relationshipmap.layout";
import { useMapTextMeasures } from "./relationshipmap.measure";
import { Node } from "./relationshipnode";

const LABELS: RelationshipMapLabels = {
  region: "Contact map for this company",
  band: { strong: "strong", developing: "developing", cold: "cold" },
  bestRoute: "Best route",
  alternatives: "Alternatives",
  noRoute: "No route recorded",
  laneMore: (hidden) => `Show ${hidden} more`,
  clearFocus: "Clear selection",
  emptyTitle: "No route recorded yet",
  emptyBody: "Start by assigning the buying roles.",
  nothingSelected: "Select a contact to see the best route to them.",
};

const PAD = 8;

function placedAt(id: string): Placed {
  return {
    id,
    x: PAD,
    y: PAD,
    w: COL_W.right,
    h: NODE_H.contact,
    kind: "contact",
    laneId: "influencer",
  };
}

function OneNode({ node }: Readonly<{ node: MapNode }>) {
  const svg = useRef<SVGSVGElement>(null);
  const measures = useMapTextMeasures(svg);
  const width = COL_W.right + PAD * 2;
  const height = NODE_H.contact + PAD * 2;
  return (
    <svg
      ref={svg}
      className="rmap-svg"
      viewBox={`0 0 ${width} ${height}`}
      width={width}
      height={height}
    >
      <title>{LABELS.region}</title>
      <Node
        placed={placedAt(node.id)}
        node={node}
        labels={LABELS}
        lane={{ label: "Influencers", hidden: 0 }}
        focused={false}
        related={false}
        faded={false}
        tabbable
        onPress={() => {}}
        measures={measures}
      />
    </svg>
  );
}

const meta = {
  title: "Components/Text and data display/Relationship map node",
  component: OneNode,
  parameters: { layout: "padded" },
  args: {
    node: {
      id: "p-1",
      kind: "contact",
      label: "Ute Sommer",
      sublabel: "CFO",
      engagement: "answered",
      engagementLabel: "Answered",
    },
  },
} satisfies Meta<typeof OneNode>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ShortName: Story = {};

export const LongName: Story = {
  args: {
    node: {
      id: "p-2",
      kind: "contact",
      label: "Benjamin Alpha-Hartmann von Oberhauser-Wittgenstein",
      sublabel: "Head of Procurement and Strategic Sourcing EMEA",
      engagement: "untried",
      engagementLabel: "Not contacted",
    },
  },
  play: async ({ canvasElement }) => {
    await document.fonts.ready;
    const box = canvasElement.querySelector(".rmap-box")?.getBBox();
    const name = canvasElement.querySelector<SVGTextElement>(".rmap-name");
    const drawn = name?.getBBox();
    await expect(name?.textContent).toMatch(/…$/);
    await expect((drawn?.x ?? 0) + (drawn?.width ?? 0)).toBeLessThanOrEqual(
      (box?.x ?? 0) + (box?.width ?? 0),
    );
  },
};
