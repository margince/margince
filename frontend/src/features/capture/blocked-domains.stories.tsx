// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { BlockedDomainsCard } from "./blocked-domains";

// The three sources side by side, because telling them apart is the card's whole
// job: a model verdict, a heuristic, and a contact who deliberately let a domain
// back in. The admitted row carries a company id — the McKinsey case, where
// unblocking re-asked the company question and one landed.
const BY_VERDICT = {
  domain: "substack.example",
  admission: "suppressed",
  reason: "a newsletter platform, not a customer",
  reason_code: "bulk_sender",
  sender_kind: "newsletter",
  source: "verdict",
  decided_at: "2026-07-30T09:12:00Z",
  company_id: null,
};
const BY_HEURISTIC = {
  domain: "expensify.example",
  admission: "suppressed",
  reason: "bulk sender: no reply address, list-unsubscribe header",
  reason_code: "bulk_sender",
  source: "heuristic",
  decided_at: "2026-08-02T14:40:00Z",
  company_id: null,
};
const BY_HUMAN = {
  domain: "mckinsey.example",
  admission: "admitted",
  reason: "they became a client in July",
  reason_code: "stated",
  note: "They became a client in July, after the newsletter publisher was acquired by a group we already sell to.",
  source: "human",
  decided_at: "2026-08-11T07:05:00Z",
  company_id: "018f3a1b-0000-7000-8000-00000000c001",
  company_name: "McKinsey and Company",
};
// A colleague made the company by hand, which let the domain in.
const BY_COLLEAGUE = {
  domain: "a-very-long-registrable-domain-for-a-client.example",
  admission: "admitted",
  reason: "a company was created on this domain",
  reason_code: "company_created",
  source: "human",
  decided_at: "2026-08-12T10:00:00Z",
  company_id: "018f3a1b-0000-7000-8000-00000000c002",
  company_name: "Brandt Automotive Zulieferteile GmbH und Co. KG",
};
// The two open questions that reach this list: the ones belonging to nobody.
// Nothing decided them, their retry cursor is cleared, and no colleague's queue
// can carry them — a question is addressed by the mailbox that raised it, and
// these have none left. Without this surface they would be visible to nobody.
const UNEVIDENCED = {
  domain: "pwc.example",
  admission: "undecided",
  reason:
    "Nothing on the site named a company, and the sender's name did not explain the domain.",
  reason_code: "unevidenced",
  source: "unevidenced",
  decided_at: "2026-08-14T11:20:00Z",
  company_id: null,
};
const STALE = {
  domain: "oldclient.example",
  admission: "undecided",
  reason:
    "The newest mail from this domain is too old to trust today's site as evidence about it.",
  reason_code: "stale_evidence",
  source: "stale_evidence",
  decided_at: "2026-08-15T06:40:00Z",
  company_id: null,
};

function story(
  entries: Record<string, unknown>[],
  total: number,
  allow: Parameters<typeof meRoute>[0],
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /capture/blocked-domains": () =>
        jsonResponse({ data: entries, total }),
      "PUT /capture/blocked-domains": (body) => jsonResponse(body),
    });
    return (
      <StoryProviders>
        <BlockedDomainsCard />
      </StoryProviders>
    );
  };
}

// Reading is every human role's; changing an entry is company:update.
const OPS = { company: ["read", "update"] } as const;
const READER = { company: ["read"] } as const;

const meta: Meta<typeof BlockedDomainsCard> = {
  title: "Settings/Data/Capture rules/Refused domains",
  component: BlockedDomainsCard,
};
export default meta;
type Story = StoryObj<typeof BlockedDomainsCard>;

export const Populated: Story = {
  render: story([BY_VERDICT, BY_HEURISTIC, BY_HUMAN, BY_COLLEAGUE], 4, OPS),
};

// The row's verbs sit behind one menu at a fixed place; an open question
// offers only a re-ask.
export const RowMenu: Story = {
  render: story([UNEVIDENCED, BY_HUMAN], 2, OPS),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for pwc.example" }),
    );
    await expect(
      await within(canvasElement.ownerDocument.body).findByRole("button", {
        name: "Ask again",
      }),
    ).toBeVisible();
    await expect(
      within(canvasElement.ownerDocument.body).queryByRole("button", {
        name: "Allow",
      }),
    ).toBeNull();
  },
};

// Decisions and open questions in one list, which is the case the card exists
// for. An operator hunting a company that never appeared has to tell "we
// refused this" from "nobody ever answered". The undecided rows carry a
// different verb — asking again, not deciding — and a neutral badge.
export const WithOpenQuestions: Story = {
  render: story([UNEVIDENCED, STALE, BY_HEURISTIC, BY_HUMAN], 4, OPS),
};

// Nothing refused yet. It has to read as a fact about the installation rather
// than as a table that failed to draw.
export const Empty: Story = {
  render: story([], 0, OPS),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const none = await canvas.findByText(/No domains refused yet/);
    await expect(none.closest(".panel-body")?.textContent).toMatch(
      /Domains refused as companies/,
    );
  },
};

// Refusals accumulate on their own from every bulk-sender verdict, so the list
// is paged at 200 and the response says how many exist. The count line under the
// table is the only thing that stops "past the end of this page" reading as
// "never refused".
export const PastTheFirstPage: Story = {
  render: story([BY_VERDICT, BY_HEURISTIC, BY_HUMAN], 412, OPS),
};

// A seat that may read the posture and not change it: the controls disable and
// say why, rather than vanishing and leaving the reader to guess whether the
// list is complete.
export const ReadOnly: Story = {
  render: story([BY_VERDICT, BY_HEURISTIC, BY_HUMAN], 3, READER),
};

// The card at 390px. A reason is a whole sentence and a domain is one unbroken
// token, so this is where the table decides which of them gets the width.
export const PopulatedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story([UNEVIDENCED, BY_HEURISTIC, BY_HUMAN, BY_COLLEAGUE], 4, OPS),
};
