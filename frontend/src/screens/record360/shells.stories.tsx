// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import type {
  SectionDetail,
  SectionState,
} from "../../design-system/surfacestate";
import { StoryProviders } from "../story-utils";
import { OverlayFallback, RailPanel, SectionCard } from "./shells";

// The two shells every record page draws its sections in, and the surface that
// replaces the page when there is nothing to assemble.
//
// Both shells exist for one distinction: "there is nothing here" and "you may
// not read this" make the SAME shape on screen and mean opposite things, so the
// states are drawn together — the only way to judge them is side by side.
//
// The second thing these stories hold is what a shell does with the caller's
// footer and verbs. A section the reader may not read, or one that failed to
// load, must not offer a button that changes it: nobody who cannot see the
// deals has business being invited to add one, and a section that never
// arrived cannot say whether the write would make sense. That rule lives in a
// boolean inside each shell and shows up nowhere else — a regression would
// draw a live "Add deal" under "hidden from you" and every test would pass.
//
// `incompleteGraph` is exported from the same module and has no story on
// purpose: it is a predicate over a graph payload, not a component, so there is
// nothing for it to render. Its behaviour belongs to the page that calls it.

const ROWS = (
  <ul className="t-body" style={{ listStyle: "none", margin: 0 }}>
    <li>Renewal 2027 — €48,000</li>
    <li>Expansion, EU — €12,500</li>
  </ul>
);

const ALL_STATES: readonly SectionState[] = [
  "ready",
  "empty",
  "withheld",
  "unavailable",
  "loading",
  "unsupported",
  "failed",
  "stale",
  "partial",
];

// Every field at once, and each state reads exactly one of them: `failed` the
// retry, `stale` the as-of, `partial` the count still missing, `unsupported`
// and `withheld` their sentence. A state whose field is absent falls back to
// the generic line, which is the floor rather than the target.
const DETAIL: SectionDetail = {
  onRetry: () => undefined,
  staleAsOf: "9:15 this morning",
  remaining: 4,
  unsupportedReason:
    "This workspace reads deals from HubSpot, and a composite section cannot be assembled from a mirror.",
  withheldReason:
    "Deal amounts on this account are readable by its owner and by finance.",
};

const GRID: CSSProperties = {
  display: "grid",
  gap: "var(--space-4)",
  gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
};

const meta: Meta = {
  title: "Records/Section shells",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The nine states with a footer figure AND a create verb passed to all of
// them, so the four that carry rows show both and the five that carry a
// message show neither. Passing the verbs to every card is what makes this
// story able to fail: a shell that stopped withholding them would look
// perfectly reasonable in isolation.
export const SectionCardEveryState: Story = {
  render: () => (
    <StoryProviders>
      <div style={GRID}>
        {ALL_STATES.map((state) => (
          <SectionCard
            key={state}
            title={`Open deals — ${state}`}
            state={state}
            emptyLabel="No open deal on this account."
            detail={DETAIL}
            footer={<p className="t-caption">€60,500 across 2 deals</p>}
            actions={<Button variant="ghost">Add deal</Button>}
          >
            {ROWS}
          </SectionCard>
        ))}
      </div>
    </StoryProviders>
  ),
};

// The plain card most callers pass: rows and nothing else. A shell that drew
// its action row unconditionally would leave an empty band under every one of
// these, which is visible only when there is nothing to put in it.
export const SectionCardWithoutVerbs: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 420 }}>
        <SectionCard
          title="Open deals"
          state="ready"
          emptyLabel="No open deal on this account."
        >
          {ROWS}
        </SectionCard>
      </div>
    </StoryProviders>
  ),
};

// The rail's own chrome: on `ready` the children are handed to Panel
// undecorated, so rows run edge to edge under the header band instead of
// sitting inside a padded body. That full-bleed shape is the reason RailPanel
// exists rather than being SectionCard with a different class.
export const RailPanelReady: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 360 }}>
        <RailPanel
          title="Who is on this deal"
          state="ready"
          emptyLabel="Nobody is named on this deal yet."
          footer={<p className="t-caption">3 named, 1 unreachable</p>}
        >
          <PanelRow>Lena Fischer — decision maker</PanelRow>
          <PanelRow>Frédéric de Gombert — economic buyer</PanelRow>
          <PanelRow>Bernd Kral — technical evaluator</PanelRow>
        </RailPanel>
      </div>
    </StoryProviders>
  ),
};

// Everything that is not `ready`, in the rail's chrome: the message is padded
// inside a PanelBody, and the card-level figure is shown only where there is
// one to report. A withheld or unavailable section has no figure — printing
// the last one it had would attach a count to rows the reader is not seeing.
export const RailPanelMessageStates: Story = {
  render: () => (
    <StoryProviders>
      <div style={GRID}>
        {ALL_STATES.filter((state) => state !== "ready").map((state) => (
          <RailPanel
            key={state}
            title={`Who is on this deal — ${state}`}
            state={state}
            emptyLabel="Nobody is named on this deal yet."
            detail={DETAIL}
            footer={<p className="t-caption">3 named, 1 unreachable</p>}
          >
            <PanelRow>Lena Fischer — decision maker</PanelRow>
          </RailPanel>
        ))}
      </div>
    </StoryProviders>
  ),
};

// What stands where the whole 360 page would be when the workspace reads from
// an incumbent mirror. The read REFUSED to assemble rather than failing, so
// this is a statement and not an error — an error plate here would send a
// reader looking for a fault that does not exist.
export const OverlayFallbackSurface: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <OverlayFallback />
      </div>
    </StoryProviders>
  ),
};
