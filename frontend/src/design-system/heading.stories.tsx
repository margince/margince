// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { HeadingSize } from "./heading";
import { Heading } from "./heading";

// Size and element are two decisions, and the stories below are the three
// cases a caller meets: the sizes on their own, the two coming apart, and the
// spacing belonging to whoever holds the headings.
const meta: Meta<typeof Heading> = {
  title: "Design System/Heading",
  component: Heading,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Heading>;

const SIZES: readonly HeadingSize[] = [
  "xxlarge",
  "xlarge",
  "large",
  "medium",
  "small",
  "xsmall",
  "xxsmall",
];

// The whole scale in one column, each rung on the element its size implies —
// so the sizes can be compared against each other rather than against whatever
// a screen happened to put them next to.
function Ladder() {
  return (
    <div style={{ display: "grid", gap: "var(--space-4)" }}>
      {SIZES.map((size) => (
        <Heading key={size} size={size}>
          {size} — Northwind Traders GmbH
        </Heading>
      ))}
    </div>
  );
}

export const AllSizes: Story = { render: () => <Ladder /> };

// A modal's title is the largest thing in the modal and still the page's `h1`
// when the modal IS the page; a heading pushed down to a `span` keeps the type
// where the outline already has a title for the passage.
function Overridden() {
  return (
    <div style={{ display: "grid", gap: "var(--space-4)" }}>
      <Heading size="medium" as="h1">
        Medium type, carried on an h1
      </Heading>
      <Heading size="medium" as="span">
        Medium type on a span, which claims no place in the outline
      </Heading>
    </div>
  );
}

export const ElementOverridden: Story = { render: () => <Overridden /> };

// A heading brings no margin, so the interval here is the stack's and only the
// stack's. Set `gap` to zero and the lines touch: nothing is holding space
// back on the heading's behalf.
function NoSpacing() {
  return (
    <div style={{ display: "grid", gap: "var(--space-2)" }}>
      <Heading size="large">The parent owns the gap</Heading>
      <p className="t-body">
        Every interval above and below a heading is declared by whatever holds
        it — a stack, a panel body, a page zone.
      </p>
      <Heading size="medium">And the next one</Heading>
    </div>
  );
}

export const SpacedByTheParent: Story = { render: () => <NoSpacing /> };
