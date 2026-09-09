// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { jsonResponse, StoryProviders } from "./story-utils";
import { TeamExceptionsPanel } from "./worklist.exceptions";

// WHAT IS GOING WRONG on this lead's team — the rows, worst first.
//
// The board beside this answers "who is carrying what" and routes to a person.
// It cannot answer this: three counts per teammate cannot say that one customer
// has waited past the target while another rep's queue is merely long.
//
// EVERY ROW SHOWS ITS BASIS, which is the column worth looking at. A lead
// disputing a line can read the rule that decided it rather than the verdict
// alone, and that is the difference between a page they trust and one they stop
// opening.
//
// The four frames are the four honest answers, and the two quiet ones are the
// point of the set:
//
//   - a clear team is the HEALTHY answer and says so in one sentence. It drew
//     a table's five column names over no rows before, which tells a reader
//     neither "the team is clear" nor anything else;
//   - a read cut short by its own bound is NOT a clear team either. The caveat
//     is drawn and the footer's count is withheld, because a floor printed as a
//     count is a wrong number in the one direction this surface must not get
//     wrong — a lead told "2" over a figure that is really 2-or-more will not
//     go looking.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

type TeamExceptionsData = components["schemas"]["TeamExceptions"];

const LENA = "00000000-0000-4000-8000-000000000001";

// A team in trouble in three different ways, so the condition column reads as
// three conditions rather than one repeated: a first reply past the policy's
// own deadline, money drifting, and work nobody has taken.
const aTeamNeedingALead: TeamExceptionsData = {
  as_of: "2026-08-31T09:00:00Z",
  exceptions: [
    {
      kind: "response_breached",
      owner: { kind: "user", id: LENA, label: "Lena Fischer" },
      subject: {
        type: "person",
        id: "00000000-0000-4000-8000-0000000000a1",
        label: "Kirsten Bauer",
      },
      since: "2026-08-28T08:12:00Z",
      consequence: "The first reply is three days past the promise.",
      threshold: "past the lead-response policy's own deadline",
    },
    {
      kind: "revenue_at_risk",
      owner: { kind: "user", id: LENA, label: "Lena Fischer" },
      subject: {
        type: "deal",
        id: "00000000-0000-4000-8000-0000000000b2",
        label: "Northstar renewal",
      },
      since: "2026-08-20T09:00:00Z",
      consequence: "€84,000 has gone quiet with three weeks to close.",
      threshold: "at or above the pipeline's median open deal",
    },
    {
      // NOBODY answers for this one, and the row says so in its own words
      // rather than naming a person. An exception a teammate is carrying and
      // one going nowhere are different news to the reader who has to act.
      kind: "unassigned",
      owner: { kind: "unassigned" },
      subject: {
        type: "lead",
        id: "00000000-0000-4000-8000-0000000000c3",
        label: "Hafen Logistik",
      },
      since: "2026-08-30T16:40:00Z",
      consequence: "Nobody has picked this up since it arrived.",
      threshold: "unowned past the intake window",
    },
  ],
  truncated: false,
};

// The panel fetches, so each frame answers its own read. `/me` is served
// because the intervention column takes the record for the READER, and a
// control built on nobody is a verb with no subject.
function stubExceptions(answer: () => Promise<Response>) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/worklist/exceptions")) {
      return answer();
    }
    if (url.includes("/me")) {
      return jsonResponse({
        user: { id: LENA, display_name: "Lena Fischer" },
      });
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

function frame(answer: () => Promise<Response>) {
  stubExceptions(answer);
  return (
    <StoryProviders>
      <TeamExceptionsPanel enabled onOwner={() => {}} />
    </StoryProviders>
  );
}

const meta: Meta<typeof TeamExceptionsPanel> = {
  title: "Records/Worklist/Team exceptions",
  component: TeamExceptionsPanel,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof TeamExceptionsPanel>;

/** Three conditions, each with the rule it was judged against and the way into
 *  whoever answers for it. The footer counts how many need this lead. */
export const ATeamNeedingALead: Story = {
  render: () => frame(async () => jsonResponse(aTeamNeedingALead)),
};

/** The healthy answer, and the one sentence it earns. No table: five column
 *  names over no rows says neither that the team is clear nor anything else. */
export const AClearTeam: Story = {
  render: () =>
    frame(async () =>
      jsonResponse({ ...aTeamNeedingALead, exceptions: [], truncated: false }),
    ),
};

/** The read stopped at its own bound, so the list is a floor. The caveat is
 *  drawn under the rows and the footer's count is withheld — a floor printed as
 *  a count is a wrong number, and a lead who took this list for the whole of it
 *  would stop looking exactly where the rest begins. */
export const CutShortByItsOwnBound: Story = {
  render: () =>
    frame(async () =>
      jsonResponse({
        ...aTeamNeedingALead,
        exceptions: aTeamNeedingALead.exceptions.slice(0, 2),
        truncated: true,
      }),
    ),
};

/** The read failed. The panel says so and offers the retry rather than drawing
 *  a team with nothing wrong on it. */
export const CouldNotBeRead: Story = {
  render: () =>
    frame(async () => jsonResponse({ title: "Upstream failed" }, 502)),
};
