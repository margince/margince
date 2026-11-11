// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { expectNextBarHoldsItsText } from "../../testing/nextbarchecks";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { expectTextFits, UNBROKEN_SUBJECT } from "../../testing/unbrokentext";
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
  headline: "This may be done: an email went to them on 2026-09-13",
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
      <CompanyNextBar
        view={data}
        onOpenTasks={() => {}}
        onOpenRecord={() => {}}
      />
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

// A promise named by a subject with no space in it wraps inside the bar,
// clear of its verbs.
const unbroken: Story = {
  render: () => (
    <Bar
      data={view({
        ...promise,
        headline: `You promised Greta Schilling ${UNBROKEN_SUBJECT}`,
        may_be_done: undefined,
      })}
    />
  ),
  play: ({ canvasElement }) =>
    expectNextBarHoldsItsText(canvasElement, UNBROKEN_SUBJECT),
};
export const UnbrokenSubjectPhone: Story = {
  ...unbroken,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const UnbrokenSubjectWide: Story = { ...unbroken, tags: ["uat-wide"] };

// The server names the verb, and a label with no space in it wraps inside the
// button and the bar.
export const UnbrokenVerbLabelPhone: Story = {
  render: () => (
    <Bar
      data={view({
        ...promise,
        may_be_done: undefined,
        recommended_action: {
          kind: "open_record",
          label: UNBROKEN_SUBJECT,
          state: "available",
          destination: {
            surface: "record",
            entity_type: "contact",
            entity_id: "p-1",
          },
        },
      })}
    />
  ),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const verb = await within(canvasElement).findByRole("button", {
      name: UNBROKEN_SUBJECT,
    });
    const bar = verb.closest(".next-bar-row");
    await expect(bar).not.toBeNull();
    if (bar) await expectTextFits(verb, bar.getBoundingClientRect());
  },
};
