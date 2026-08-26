// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { FocusLane } from "./today.focus";
import type { AttentionItem } from "./today.queries";

// The day's decision lane: ONE decision on screen, its position in the queue,
// and the way past it.
//
// The lane takes what it draws as props, so a story is a queue state rather
// than a fetch — except the staged proposal, which fetches the one approval it
// is deciding, and that read's own three answers (in flight, unusable, ready)
// are three of the stories below.
//
// Two numbers sit on this panel and they are not the same kind of number. The
// badge is a MAGNITUDE — how much work is waiting — and it is written in the
// reader's own notation. The progress line beside it is a POSITION, "3 of 90",
// and it is a magnitude too: both halves count items, and a reader comparing
// "3 of 1204" against a badge reading "1.204" would be looking at one queue
// written two ways. `QueueOfManyGerman` is where that is visible at all, because
// de-DE first groups at four digits.

const PAIR: AttentionItem = {
  id: "dc-1",
  source: "dedupe_candidate",
  kind: "organization",
  confidence: 0.92,
  actions: ["merge"],
  pair: {
    left: {
      id: "org-1",
      label: "Acme Logistik GmbH",
      detail: "acme.example",
    },
    right: {
      id: "org-2",
      label: "Acme Logistik",
      detail: "acme-log.example",
    },
    evidence: [
      {
        field: "display_name",
        signal: "collide",
        left_value: "Acme Logistik GmbH",
        right_value: "Acme Logistik",
      },
      {
        field: "email",
        signal: "one_sided",
        left_value: "kontakt@acme.example",
        right_value: null,
      },
    ],
  },
};

const STAGED: AttentionItem = {
  id: "ap-1",
  source: "approval",
  kind: "send_email",
  title: "Send the follow-up to Anna Weber at Baqend",
  due_at: "2026-08-26T09:00:00Z",
  actions: ["decide"],
};

const APPROVAL = {
  id: "ap-1",
  kind: "send_email",
  status: "pending",
  summary: "Send the follow-up to Anna Weber at Baqend",
  subject_type: "person",
  subject_id: "p-1",
  payload: {
    to: "anna.weber@baqend.example",
    subject: "Following up on the retrofit window",
    body: "Anna — the depot window moved to the 14th. Can you confirm the quote before then?",
  },
  staged_by: "agent:outreach",
  autonomy_tier: "propose",
  source: "agent",
  captured_by: "agent:outreach",
  version: 1,
  created_at: "2026-08-25T08:00:00Z",
  updated_at: "2026-08-25T08:00:00Z",
};

// `/me` and the approval read are routed; everything else this lane's row
// reaches for falls through to the stub's empty page, which is a legitimate
// answer for each and keeps the story about the decision rather than its chrome.
function lane(
  args: Readonly<{
    items: readonly AttentionItem[];
    total: number;
    decided: number;
    withheld?: boolean;
    approval?: () => Response;
    locale?: "de";
  }>,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        organization: ["read", "update"],
        person: ["read", "update"],
        automation: ["read", "update"],
      }),
      "GET /approvals/ap-1": args.approval ?? (() => jsonResponse(APPROVAL)),
    });
    return (
      <StoryProviders locale={args.locale}>
        <FocusLane
          items={args.items}
          total={args.total}
          decided={args.decided}
          withheld={args.withheld ?? false}
          onDecided={() => {}}
          onSkip={() => {}}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof FocusLane> = {
  title: "Records/Today/Decision lane",
  component: FocusLane,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof FocusLane>;

/** A duplicate pair, decided in place: the evidence the detector saw, and the
 *  two verbs that answer it. */
export const AMergeToDecide: Story = {
  render: lane({ items: [PAIR], total: 4, decided: 1 }),
};

/** A staged proposal. The row that decides it is the same one the record page
 *  draws, so an editing reader meets one spelling of the decision, not two. */
export const AStagedProposal: Story = {
  render: lane({ items: [STAGED], total: 4, decided: 1 }),
};

/** The proposal's own read, in flight. The lane is already drawn around it —
 *  the position and the way past it do not wait on the card. */
export const StagedProposalLoading: Story = {
  render: lane({
    items: [STAGED],
    total: 4,
    decided: 1,
    approval: () => new Response(null, { status: 204 }),
  }),
};

/**
 * A proposal that came back without a `kind`. The card cannot draw it — the
 * kind chooses the label, the tool chip and the autonomy dot — so it reads as a
 * failed read with a retry, rather than throwing and taking the whole day's
 * surface down over one malformed answer.
 */
export const StagedProposalUnusable: Story = {
  render: lane({
    items: [STAGED],
    total: 4,
    decided: 1,
    approval: () => jsonResponse({ ...APPROVAL, kind: undefined }),
  }),
};

/** Nothing left to answer, and what the reader got through says so. */
export const AllClear: Story = {
  render: lane({ items: [], total: 0, decided: 7 }),
};

/**
 * A reader whose scope does not admit what is waiting. Withheld is not empty:
 * the lane says the queue is not theirs to see rather than telling them there
 * is nothing in it, which would be a false statement about the day.
 */
export const Withheld: Story = {
  render: lane({ items: [], total: 12, decided: 0, withheld: true }),
};

/** A queue wide enough to be written in a notation. Four digits is where de-DE
 *  first groups, so this is the narrowest queue at which badge and progress can
 *  be caught disagreeing. */
export const QueueOfMany: Story = {
  render: lane({ items: [PAIR], total: 1204, decided: 3 }),
};

/** The same queue in German. */
export const QueueOfManyGerman: Story = {
  render: lane({ items: [PAIR], total: 1204, decided: 3, locale: "de" }),
};

/** At 390px the evidence rows stack and the two merge verbs cannot share a line
 *  with "Later". */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: lane({ items: [PAIR], total: 1204, decided: 3 }),
};
