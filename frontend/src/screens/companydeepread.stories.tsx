// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DeepReadPanel } from "./companydeepread";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Reading a company's own website, end to end. The panel is an OFFER until a
// read exists and a REPORT afterwards, and the frames are that turn plus the
// outcomes a report can carry.
//
// The report is the transparency surface, so the states worth a picture are the
// ones where "it worked" and "it stopped" have to read differently: a crawl
// that spent a configured ceiling did what an operator asked for, while one
// that lost its model budget is a warning a reader can act on.

type SiteReadReport = components["schemas"]["SiteReadReport"];

const COMPANY = "01a04298-1971-7076-8076-8064da20fdff";
const READ = "01a04298-1971-7076-8076-8064da20fe00";

function report(over: Partial<SiteReadReport> = {}): SiteReadReport {
  return {
    read_id: READ,
    company_id: COMPANY,
    seed_url: "https://brandt.example",
    status: "running",
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    pages: [
      { url: "https://brandt.example/", kind: "home" },
      { url: "https://brandt.example/team", kind: "team" },
    ],
    skipped: [],
    proposal_ids: [],
    created_at: "2026-07-17T08:00:00Z",
    ...over,
  } as SiteReadReport;
}

const LATEST = `GET /companies/${COMPANY}/site-reads/latest`;
const REPORT = `GET /companies/${COMPANY}/site-reads/${READ}`;

// 404 is the honest "never read": a read id lives only in the tab that started
// the crawl, so the panel asks the server rather than its own memory.
const NEVER_READ: RouteMap = {
  [LATEST]: () => new Response(null, { status: 404 }),
};

function panel(routes: RouteMap) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <DeepReadPanel companyId={COMPANY} />
      </StoryProviders>
    );
  };
}

function reporting(over: Partial<SiteReadReport>) {
  const answer = () => jsonResponse(report(over));
  return panel({ [LATEST]: answer, [REPORT]: answer });
}

const meta: Meta<typeof DeepReadPanel> = {
  title: "Records/Company 360/Website deep read",
  component: DeepReadPanel,
};
export default meta;
type Story = StoryObj<typeof DeepReadPanel>;

/** An account nobody has researched. The panel is still a pitch: what the read
 *  does, and the two sentences saying nothing is written until a reader accepts
 *  it. */
export const NeverRead: Story = { render: panel(NEVER_READ) };

/** A crawl under way, as the two stages the engine actually reports — one
 *  finished, one running. A ladder with invented rungs would be a progress bar
 *  moving on its own schedule. */
export const Reading: Story = { render: reporting({ phase: "extracting" }) };

/** Waiting on AI budget. Still a read in flight, so the ladder stays, and the
 *  note beside it says when it resumes rather than leaving a reader guessing. */
export const WaitingForBudget: Story = {
  render: reporting({
    status: "deferred",
    status_code: "budget_deferred",
    status_detail: "The AI budget for this month is spent.",
    next_attempt_at: "2026-08-01T06:00:00Z",
    phase: null,
  }),
};

/** Finished, with facts staged for review. The dot and the count are the
 *  handover: nothing was written to the record, and the inbox is where a reader
 *  decides. */
export const Done: Story = {
  render: reporting({
    status: "done",
    phase: null,
    fact_count: 14,
    proposal_ids: ["01a04298-1971-7076-8076-8064da20ff01"],
  }),
};

/** A crawl that spent its configured page budget. It is named for what it DID,
 *  not for what it stopped short of, and it draws no warning — a ceiling an
 *  operator set is not a fault a reader can repair. */
export const ReadUpToThePageLimit: Story = {
  render: reporting({
    status: "partial",
    stopped_reason: "page_cap",
    phase: null,
    fact_count: 6,
  }),
};

/** A crawl that lost its model budget. This one a reader CAN act on, so it is
 *  the stop that earns the warn badge. */
export const StoppedEarly: Story = {
  render: reporting({
    status: "partial",
    stopped_reason: "budget",
    phase: null,
    fact_count: 2,
  }),
};

/** The crawl failed. Danger tone on the status, and still no page telling the
 *  reader what our internals said. */
export const Failed: Story = {
  render: reporting({ status: "failed", phase: null }),
};
