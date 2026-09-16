// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { momentFallbackVerb } from "./companytodayverbs";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The leading card's fallback verb column — what the account offers when the
// moment above it named no destination of its own.
//
// It is drawn off the account's own state rather than off the moment, so the
// frames below are the three answers that state can give: a task already on
// the list, a reply we owe, and a reply we are waiting for. Each is a real arm
// of the same function; none is a variant of a control.
//
// The column is a fragment of `.today-verb` items, so the story supplies the
// `.today-actions` column its row would — without it the verbs draw in a bare
// inline flow with no gap, which is a layout nobody ships.

type Company360 = components["schemas"]["Company360"];
type Engagement = NonNullable<
  NonNullable<Company360["state_strip"]>["engagement"]
>["state"];

const company: components["schemas"]["Company"] = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const page = { has_more: false, next_cursor: null };

const contact: components["schemas"]["Company360Contact"] = {
  contact_id: "p-1",
  full_name: "Greta Schilling",
  title: "Head of Procurement",
  strength: {
    score: 71,
    bucket: "strong",
    factors: {
      recency: 0.8,
      frequency: 0.6,
      reciprocity: 0.7,
      direction: 0.5,
    },
  },
  deal_roles: [],
  // Every purpose reads `unknown` until a row says otherwise, which is
  // default-deny for outbound rather than "not applicable".
  consent: {},
};

function view(
  over: Partial<Company360> & { engagement?: Engagement } = {},
): Company360 {
  const { engagement, ...rest } = over;
  return {
    as_of: "2026-09-01T09:00:00Z",
    company,
    sections_omitted: [],
    contacts: { data: [contact], page },
    state_strip: {
      account: { lifecycle: "customer", relationship_types: ["customer"] },
      ...(engagement ? { engagement: { state: engagement } } : {}),
    },
    ...rest,
  };
}

function Verbs({ data }: Readonly<{ data: Company360 }>) {
  installFetchStub({ "GET /me": meRoute({ company: ["read"] }) });
  return (
    <StoryProviders>
      <Column data={data} />
    </StoryProviders>
  );
}

// Inside the providers, because the column is built by a function that reads
// the catalog rather than by a component that could be mounted directly.
function Column({ data }: Readonly<{ data: Company360 }>) {
  const t = useT();
  return (
    <span className="today-actions">
      {momentFallbackVerb({
        view: data,
        t,
        onOpenTask: () => {},
        onDraftTo: () => {},
        onLogActivity: () => {},
      })}
    </span>
  );
}

const meta: Meta = {
  title: "Records/Company record/Moment fallback verbs",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// A task already on the account's list outranks a reply, whatever is owed: it
// is the more specific of the two, named by an activity rather than inferred.
export const TaskOnTheList: Story = {
  render: () => (
    <Verbs
      data={view({
        engagement: "waiting_on_us",
        next_steps: {
          data: [
            {
              activity_id: "a-1",
              subject: "Send the revised retrofit quote",
              due_at: "2026-09-02T16:00:00Z",
              overdue: false,
            },
          ],
          page,
        },
      })}
    />
  ),
};

// Nothing on the list and the reply is ours to write: the verb goes to the
// strongest contact on the account, with logging beside it.
export const WeOweTheReply: Story = {
  render: () => <Verbs data={view({ engagement: "waiting_on_us" })} />,
};

// Waiting on them, so the same destination reads as a follow-up rather than as
// an answer — the word is the only thing that differs, and it is the word a rep
// judges before pressing.
export const WaitingOnThem: Story = {
  render: () => <Verbs data={view({ engagement: "waiting_on_them" })} />,
};
