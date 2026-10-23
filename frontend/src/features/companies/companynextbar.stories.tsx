// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyNextBar } from "./companynextbar";

// The account's next bar when its moment is a promise we made. A later mail
// of ours to the same contact on the same thread turns the verb into the
// "Done?" question; without that mail the bar keeps the moment's own verb.

type Company360 = components["schemas"]["Company360"];
type Moment = NonNullable<Company360["moment"]>;

const company: components["schemas"]["Company"] = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const page = { has_more: false, next_cursor: null };

const promise: Moment = {
  claim_key: "moment:account_may_be_done:c-3",
  evidence_fingerprint: "fp-1",
  rule: "open_promise",
  headline: "You may have done this — you wrote to them on 2026-09-13",
  why_now:
    "You owe Greta Schilling: Send the listing details. Promised with no due date.",
  confidence: "medium",
  evidence: [
    { type: "activity", id: "e-0", label: "Send the listing details" },
    { type: "activity", id: "e-1", label: "Re: Listing details" },
  ],
  recommended_action: {
    kind: "complete_task",
    label: "Done",
    state: "available",
  },
  may_be_done: {
    promise_type: "claim",
    promise_id: "c-3",
    email_activity_id: "e-1",
    wrote_at: "2026-09-13T09:00:00Z",
  },
};

function view(moment: Moment): Company360 {
  return {
    as_of: "2026-09-15T09:00:00Z",
    company,
    sections_omitted: [],
    contacts: { data: [], page },
    next_steps: { data: [], page },
    moment,
  };
}

function Bar({ data }: Readonly<{ data: Company360 }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read"], contact: ["read", "update"] }),
  });
  return (
    <StoryProviders>
      <CompanyNextBar view={data} onOpenTasks={() => {}} />
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Company 360/Next bar",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// We wrote to them on the promise's thread after it was made: the bar asks
// whether that kept it, with Done and Not yet.
export const AsksWhetherAPromiseWasKept: Story = {
  render: () => <Bar data={view(promise)} />,
};

// The same promise with no later mail of ours: the bar offers the moment's
// own verb and asks nothing.
export const APromiseWithNoLaterMail: Story = {
  render: () => (
    <Bar
      data={view({
        ...promise,
        headline: "You promised Greta Schilling the listing details",
        may_be_done: undefined,
      })}
    />
  ),
};
