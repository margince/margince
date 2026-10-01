// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Button, Card } from "./atoms";

const meta: Meta<typeof Card> = {
  title: "Components/Layout and structure/Card",
  component: Card,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Card>;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

// The header is props rather than a hand-placed child, so title and actions
// sit where every other card puts them.
export const Surfaces: Story = {
  render: () => (
    <div style={stack}>
      <Card>
        <p className="t-caption">
          The standing surface: a card carries a section of a record.
        </p>
      </Card>
      <Card inset>
        <p className="t-caption">
          The inset variant sits inside another surface, so it recedes instead
          of stacking a second raised edge on the first.
        </p>
      </Card>
      <Card title="Passports" actions={<Button>Mint</Button>}>
        <p className="t-caption">
          The header comes from props: title over description across the full
          width, actions beside the pair.
        </p>
      </Card>
    </div>
  ),
};
