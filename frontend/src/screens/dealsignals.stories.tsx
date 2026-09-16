// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { Panel, PanelBody } from "../design-system/panel";
import { useDealSignals } from "./dealsignals";
import { SignalStrip } from "./record360";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type DealCoverage = components["schemas"]["DealCoverage"];

// The deal's coverage findings as the chips a rep scans above the reading —
// `useDealSignals` over the coverage card's own cached query, drawn through
// `SignalStrip` exactly as Deal360's call card draws it.
//
// The subject is a HOOK, so the harness below is the story's own frame: the
// panel and the readout line are not a surface the deal page has. The readout
// earns its place because the hook's whole reason for existing is the answer
// the strip cannot draw — a strip renders nothing for "no findings" and
// nothing for "you were served none", and reporting a clean bill of health
// from a check that never ran is the failure `withheld` prevents.
//
// `days_since_touch` rides `going_cold` and no other kind (DealCoverageRisk),
// so exactly one chip here carries a figure. A fixture that put a number on
// every chip would picture a strip the server cannot send.

const DEAL_ID = "d-brandt";

function coverage(over: Partial<DealCoverage>): DealCoverage {
  return {
    deal_id: DEAL_ID,
    stakeholders: [],
    our_side: [],
    risks: [],
    sections_omitted: [],
    ...over,
  };
}

// One finding of each loudness: the two kinds that mean somebody is GONE are
// danger, everything else is a warning, and only the cold one has a number.
const FINDINGS: DealCoverage["risks"] = [
  {
    kind: "champion_left",
    summary: "Frédéric Weber left Brandt Automotive on 12 August.",
  },
  {
    kind: "going_cold",
    summary: "No captured touch on this deal since 24 June.",
    days_since_touch: 84,
  },
  {
    kind: "single_threaded_theirs",
    summary: "Only one contact at Brandt is engaged on this deal.",
  },
];

/** What the hook reported, when it reported no chips. */
function readout(withheld: boolean, ready: boolean): string {
  if (!ready) {
    return "Reading this deal's coverage.";
  }
  return withheld
    ? "Withheld: without the relationship grant this reader is served no findings, so no check ran."
    : "The check ran and found nothing wrong with this deal's coverage.";
}

function DealSignals({ dealId }: Readonly<{ dealId: string }>) {
  const { signals, withheld, ready } = useDealSignals(dealId);
  return (
    <Panel tone="ai" title="Brandt Automotive GmbH · Renewal">
      <SignalStrip signals={signals} />
      {signals.length === 0 ? (
        <PanelBody>
          <p className="t-caption">{readout(withheld, ready)}</p>
        </PanelBody>
      ) : null}
    </Panel>
  );
}

function strip(body: DealCoverage) {
  return () => {
    installFetchStub({
      [`GET /deals/${DEAL_ID}/coverage`]: () => jsonResponse(body),
    });
    return (
      <StoryProviders>
        <DealSignals dealId={DEAL_ID} />
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  title: "Records/Deal signals",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** Both tones side by side, and the one chip that states its own number. */
export const Findings: Story = { render: strip(coverage({ risks: FINDINGS })) };

// Dark is where the strip is worth a second look: `danger` and `warning` are
// two soft badges in a row, and the accent lift has to leave them tellable
// apart at a glance or the loud finding stops being the loud one.
export const FindingsDark: Story = {
  globals: { theme: "dark" },
  render: strip(coverage({ risks: FINDINGS })),
};

/** The check ran and nothing tripped — no chips, and the page says why. */
export const NothingWrong: Story = { render: strip(coverage({})) };

// Withheld, which is NOT the story above it: the contract omits the three
// coverage sections together for a reader without the relationship grant.
export const Withheld: Story = {
  render: strip(
    coverage({ sections_omitted: ["stakeholders", "our_side", "risks"] }),
  ),
};
