// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Lock, Mail } from "lucide-react";
import type { CSSProperties } from "react";
import { BADGE_TONES, Badge } from "./atoms";
import { Heading } from "./heading";

const meta: Meta<typeof Badge> = {
  title: "Components/Labels/Badge",
  component: Badge,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Badge>;

const row: CSSProperties = {
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  flexWrap: "wrap",
};
const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

const BADGE_VARIANTS = ["soft", "primary"] as const;
// Walked from the component's own tones, so the newest tone is drawn the day it
// lands; `default` heads each row on its own.
const BADGE_TINTS = BADGE_TONES.filter((tone) => tone !== "default");
const badgeDocs = (story: string) => ({ docs: { description: { story } } });

export const Variants: Story = {
  parameters: badgeDocs(`A badge states one status or label fact beside a name.
- **Soft** is the default. **Primary** is for the one status a reader must
  not miss, and for counts. One variant per context, never mixed in a column.
- An icon sits LEFT of the label, never right; a badge has no trailing slot.
- Soft has a tone hairline, primary none; add no border, caps or pill class.
- Don't make a badge interactive: a pressable fact is \`Chip\`, a filter is
  \`FilterPills\`, a verb is \`Button\`.
- No new colours. \`ai\` (always with Sparkles) means an agent proposed it, and
  \`discovery\` means something new rather than something going well.`),
  render: () => (
    <div style={stack}>
      {BADGE_VARIANTS.map((variant) => (
        <div key={variant} style={row}>
          <Badge variant={variant}>default</Badge>
          {BADGE_TINTS.map((tone) => (
            <Badge key={tone} variant={variant} tone={tone}>
              {tone}
            </Badge>
          ))}
        </div>
      ))}
    </div>
  ),
};

export const WithIcon: Story = {
  parameters: badgeDocs("A glyph names the kind of status; ai's is Sparkles."),
  render: () => (
    <div style={stack}>
      {BADGE_VARIANTS.map((variant) => (
        <div key={variant} style={row}>
          <Badge variant={variant} tone="success" icon={Mail}>
            Replied
          </Badge>
          <Badge variant={variant} tone="danger" icon={Lock}>
            Restricted
          </Badge>
          <Badge variant={variant} tone="ai">
            Drafted
          </Badge>
        </div>
      ))}
    </div>
  ),
};

export const Live: Story = {
  parameters: badgeDocs("Happening now; the dot stops under reduced motion."),
  render: () => (
    <div style={row}>
      <Badge tone="success" live>
        Live
      </Badge>
      <Badge variant="primary" tone="accent" live>
        Publishing
      </Badge>
    </div>
  ),
};

export const LongLabel: Story = {
  parameters: badgeDocs("At 200px, alone and beside a sibling: an ellipsis."),
  render: () => (
    <div style={{ ...stack, alignItems: "flex-start", inlineSize: 200 }}>
      <Badge tone="warning">
        extensions/acme/routes/partner-portal/settings
      </Badge>
      <div style={{ ...row, flexWrap: "nowrap", inlineSize: "100%" }}>
        <span>Route</span>
        <Badge icon={Lock}>extensions/acme/routes/partner-portal</Badge>
      </div>
    </div>
  ),
};

export const InsideUppercaseParent: Story = {
  parameters: badgeDocs("A parent's case, tracking and face stop at its edge."),
  render: () => (
    <div style={stack}>
      <Heading size="medium" className="t-eyebrow">
        Pipeline <Badge tone="accent">Three open</Badge>
      </Heading>
      <code>
        run 4f2a <Badge tone="success">Passed</Badge>
      </code>
    </div>
  ),
};
