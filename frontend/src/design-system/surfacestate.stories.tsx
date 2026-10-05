// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Card } from "./atoms";
import { Eyebrow } from "./eyebrow";
import { Panel, PanelBody } from "./panel";
import { type SectionState, SurfaceState } from "./surfacestate";

// The nine states a surface can be in. They are drawn together because that is
// the only way to judge the thing they exist for: an empty card and a withheld
// card make the same shape on screen and mean opposite things, and the words
// are all that separate them.
const meta: Meta<typeof SurfaceState> = {
  title: "Components/Messaging/Surface state",
  component: SurfaceState,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof SurfaceState>;

const ROWS = (
  <ul className="t-body" style={{ listStyle: "none" }}>
    <li>Renewal — €48,000</li>
    <li>Expansion, EU — €12,500</li>
  </ul>
);

const ALL: readonly SectionState[] = [
  "ready",
  "empty",
  "withheld",
  "unavailable",
  "loading",
  "failed",
  "stale",
  "partial",
];

const DETAIL = {
  onRetry: () => undefined,
  staleAsOf: "9:15 this morning",
  remaining: 4,
};

function AllStates({ label }: Readonly<{ label?: string }>) {
  return (
    <div
      style={{
        display: "grid",
        gap: "var(--space-4)",
        gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))",
      }}
    >
      {ALL.map((state) => (
        <Card key={state} title={state}>
          <SurfaceState
            label={label}
            loadingLabel="Loading the section"
            state={state}
            emptyLabel="No open deals for this company."
            detail={DETAIL}
          >
            {ROWS}
          </SurfaceState>
        </Card>
      ))}
    </div>
  );
}

export const EveryState: Story = { render: () => <AllStates /> };

/** The same states under a part's own heading: every sentence sits one step
 * under it, so "none" and "hidden" are not told apart by where they sit. */
export const EveryStateNamed: Story = {
  render: () => <AllStates label="Deals" />,
};

/** `stale` puts the caveat ABOVE the rows and `partial` puts the count BELOW
 * them, and neither is a layout preference: a caveat under a figure arrives
 * after the reader has already taken it as current, while a truncation count
 * above a list describes something they have not read yet. */
function OrderDemo() {
  return (
    <div style={{ display: "grid", gap: "var(--space-4)", maxWidth: 420 }}>
      <Card title="Stale — caveat first">
        <SurfaceState
          loadingLabel="Loading the section"
          state="stale"
          emptyLabel="No open deals for this company."
          detail={{ staleAsOf: "9:15 this morning" }}
        >
          {ROWS}
        </SurfaceState>
      </Card>
      <Card title="Partial — count last">
        <SurfaceState
          loadingLabel="Loading the section"
          state="partial"
          emptyLabel="No open deals for this company."
          detail={{ remaining: 4 }}
        >
          {ROWS}
        </SurfaceState>
      </Card>
    </div>
  );
}

export const CaveatOrder: Story = { render: () => <OrderDemo /> };

/** A `failed` state with no `onRetry` is `unavailable` with extra words, so it
 * draws the sentence and no button — the retry is what makes the state
 * different, not the wording. */
function RetryDemo() {
  return (
    <div style={{ display: "grid", gap: "var(--space-4)", maxWidth: 420 }}>
      <Card title="Failed, retryable">
        <SurfaceState
          loadingLabel="Loading the section"
          state="failed"
          emptyLabel="Nothing recorded."
          detail={{ onRetry: () => undefined }}
        >
          {ROWS}
        </SurfaceState>
      </Card>
      <Card title="Failed, nothing to retry">
        <SurfaceState
          state="failed"
          emptyLabel="Nothing recorded."
          loadingLabel="Loading the section"
        >
          {ROWS}
        </SurfaceState>
      </Card>
    </div>
  );
}

export const FailedWithAndWithoutRetry: Story = {
  render: () => <RetryDemo />,
};

/** Two independently-governed parts in one card. Each is named, so "hidden
 * from you" attaches to a named thing rather than floating under a heading
 * that covers both. */
function LabelledDemo() {
  return (
    <Card title="Tags and lists" style={{ maxWidth: 420 }}>
      <SurfaceState
        label="Lists"
        state="ready"
        emptyLabel="Not on any list."
        loadingLabel="Loading the section"
      >
        <p className="t-body">Q3 expansion targets</p>
      </SurfaceState>
      <SurfaceState
        label="Tags"
        state="withheld"
        emptyLabel="No tags."
        loadingLabel="Loading the section"
      >
        <p className="t-body">strategic</p>
      </SurfaceState>
    </Card>
  );
}

export const NamedParts: Story = { render: () => <LabelledDemo /> };

/** A card whose parts are themselves sections of a bigger card. The labels
 * drop to h4 so the outline NESTS — an h3 under an h3 reads to a screen
 * reader as a sibling of the section it belongs to, which is a flat list of
 * everything on the page rather than a structure a reader can walk. */
function NestedDemo() {
  return (
    <Card title="Company 360" style={{ maxWidth: 420 }}>
      <Eyebrow as="h3">What is in flight</Eyebrow>
      <SurfaceState
        loadingLabel="Loading the section"
        label="Deals"
        labelLevel="h4"
        state="ready"
        emptyLabel="No open deals."
      >
        <p className="t-body">Fleet retrofit 2026</p>
      </SurfaceState>
      <SurfaceState
        loadingLabel="Loading the section"
        label="Projects"
        labelLevel="h4"
        state="withheld"
        emptyLabel="No projects in flight."
      >
        <p className="t-body">Depot fit-out</p>
      </SurfaceState>
    </Card>
  );
}

export const NestedUnderASection: Story = { render: () => <NestedDemo /> };

// The stack's gap falls under the pair, never between its two lines.
export const EmptyWithDetailInAStack: Story = {
  render: () => (
    <Card>
      <div className="form-stack">
        <SurfaceState
          state="empty"
          emptyLabel="No renewals on this account."
          emptyDetail="A renewal appears once a deal is marked as one."
          loadingLabel="Loading renewals"
        >
          {ROWS}
        </SurfaceState>
        <p className="t-body">Next review in March.</p>
      </div>
    </Card>
  ),
};

// The caveat stays on its figures under flow spacing and under a gap stack.
export const StaleInBothHosts: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-4)", maxWidth: 420 }}>
      <Panel title="Panel body">
        <PanelBody>
          <SurfaceState
            loadingLabel="Loading the section"
            state="stale"
            emptyLabel="No open deals for this company."
            detail={{ staleAsOf: "9:15 this morning" }}
          >
            {ROWS}
          </SurfaceState>
        </PanelBody>
      </Panel>
      <Card title="Form stack">
        <div className="form-stack">
          <SurfaceState
            loadingLabel="Loading the section"
            state="stale"
            emptyLabel="No open deals for this company."
            detail={{ staleAsOf: "9:15 this morning" }}
          >
            {ROWS}
          </SurfaceState>
          <p className="t-body">Next review in March.</p>
        </div>
      </Card>
    </div>
  ),
};
