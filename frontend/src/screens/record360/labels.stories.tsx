// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { StoryProviders } from "../story-utils";
import { signalKindLabel, signalTone } from "./labels";

// Every signal kind, in the reader's words.
//
// This exists because the failure it catches is invisible in a screenshot of
// any ONE signal: a kind the map never learned renders as its own wire value —
// `product_launch` rather than "Neues Produkt" — which looks like a data
// problem rather than a missing label, and only shows up on an account that
// happens to have that kind of news.
//
// Rendered as a list so an unlabelled kind is obvious beside its labelled
// neighbours.

const meta: Meta = {
  title: "Records/Company 360/Signal labels",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// Every kind a producer can raise, in the order the vocabulary grew: the six a
// human files by hand, the five the deterministic producers raise, the four a
// company's own newsroom announces, and what a technical lookup notices.
const KINDS = [
  "stalled_deal",
  "champion_left",
  "reengagement",
  "buying_intent",
  "risk",
  "other",
  "contract_ended",
  "new_opportunity",
  "commitment_made",
  "ghosted_thread",
  "project_gone_quiet",
  "funding",
  "leadership_change",
  "expansion",
  "product_launch",
  "technical_change",
];

function SignalKinds() {
  const t = useT();
  return (
    <div style={{ display: "grid", gap: "var(--space-2)" }}>
      {KINDS.map((kind) => (
        <div
          key={kind}
          style={{
            display: "flex",
            alignItems: "center",
            gap: "var(--space-3)",
          }}
        >
          <Badge tone={signalTone("info")} quiet>
            {signalKindLabel(kind, t)}
          </Badge>
          <span className="t-caption">{kind}</span>
        </div>
      ))}
    </div>
  );
}

/** German, the default locale: a kind whose label is missing shows its raw wire
 * value in the badge, beside the same value in the caption. */
export const German: Story = {
  render: () => (
    <StoryProviders locale="de">
      <SignalKinds />
    </StoryProviders>
  ),
};

export const English: Story = {
  render: () => (
    <StoryProviders locale="en">
      <SignalKinds />
    </StoryProviders>
  ),
};
