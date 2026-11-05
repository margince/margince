// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Plus } from "lucide-react";
import { Button } from "./atoms";
import { Heading } from "./heading";
import { JumpBar } from "./jumpbar";

/**
 * The section pills of a long page, over sections tall enough to scroll. A
 * press shows the jump, and the bar stays pinned above it.
 */
const TARGETS = [
  { id: "jb-voice", label: "Your voice" },
  { id: "jb-moves", label: "Signature moves" },
  { id: "jb-sharpen", label: "Make it sharper" },
  { id: "jb-builds", label: "Builds" },
];

const meta: Meta<typeof JumpBar> = {
  title: "Components/Navigation/Jump bar",
  component: JumpBar,
};
export default meta;
type Story = StoryObj<typeof JumpBar>;

function Page({ action }: Readonly<{ action?: boolean }>) {
  return (
    <div style={{ height: "28rem", overflow: "auto", padding: "1rem" }}>
      <JumpBar
        label="On this page"
        targets={TARGETS}
        action={
          action ? (
            <Button variant="primary">
              <Plus aria-hidden /> Add writing
            </Button>
          ) : undefined
        }
      />
      {TARGETS.map((target) => (
        <section
          key={target.id}
          id={target.id}
          tabIndex={-1}
          style={{ minHeight: "20rem", paddingTop: "1rem" }}
        >
          <Heading size="medium" as="h2">
            {target.label}
          </Heading>
        </section>
      ))}
    </div>
  );
}

export const Sections: Story = { render: () => <Page /> };

export const WithAction: Story = { render: () => <Page action /> };

export const Phone: Story = {
  tags: ["uat-phone"],
  render: () => <Page action />,
};
