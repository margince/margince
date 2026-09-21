// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import { StoryProviders } from "./story-utils";
import { VerdictLine } from "./worklist.verdict";
// The line is drawn INSIDE a queue row, and everything about its measure — the
// badge sitting on the sentence's baseline, the reading date trailing it — is
// the row's stylesheet rather than this component's.
import "./worklist.row.css";

// What the queue says the deal is doing, under the step that acts on it.
//
// Two frames, because the server sends two kinds of line and the difference is
// the whole reason this component exists: a cached deal card carries one of the
// four standings and draws its badge, while the night's brief finding is prose
// about the deal and carries none. A badge invented for the second would be the
// queue deciding a judgement the deal page owns.

type WorklistDealVerdict = components["schemas"]["WorklistDealVerdict"];

function Lines({
  verdicts,
}: Readonly<{ verdicts: readonly WorklistDealVerdict[] }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        {verdicts.map((verdict) => (
          <VerdictLine
            key={verdict.line}
            verdict={verdict}
            zone={viewerZone()}
          />
        ))}
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Worklist/Verdict line",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The four standings together, which is the only way to judge them: `cold`
// takes no tone on purpose, and whether that reads as a statement of fact
// rather than as a fourth instruction can only be seen beside the three that
// do carry one.
export const FourStandings: Story = {
  render: () => (
    <Lines
      verdicts={[
        {
          standing: "live",
          line: "Both sides have the retrofit walkthrough on Thursday.",
          source: "deal_status",
          as_of: "2026-09-01T06:12:00Z",
        },
        {
          standing: "drifting",
          line: "Nothing has moved since the quote went out three weeks ago.",
          source: "deal_status",
          as_of: "2026-09-01T06:12:00Z",
        },
        {
          standing: "blocked",
          line: "Procurement is waiting on the security questionnaire.",
          source: "deal_status",
          as_of: "2026-08-28T06:12:00Z",
        },
        {
          standing: "cold",
          line: "No reply since the March site visit.",
          source: "deal_status",
          as_of: "2026-08-12T06:12:00Z",
        },
      ]}
    />
  ),
};

// The night's finding, which carries no standing and no badge — and no reading
// instant either, so the sentence stands alone on the row.
export const BriefFinding: Story = {
  render: () => (
    <Lines
      verdicts={[
        {
          line: "Their new plant manager was announced on Monday; the retrofit scope is likely to be re-drawn.",
          source: "brief_finding",
        },
      ]}
    />
  ),
};
