// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../../design-system/panel";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DomainQuestionAnswer } from "./worklist.domainquestion";
import type { WorklistItem } from "./worklist.queries";
// The row's own line. `.worklist-row-acts` carries the trailing alignment and
// the interval between the two answers; without it they draw as a bare inline
// run with no gap, which is a picture of markup rather than of the row.
import "./worklist.row.css";

// ANSWERING AN UNDECIDED DOMAIN, from the row that asked.
//
// No verb is filled, and none takes the end of the line. The machine could not
// tell which company this is, and a filled verb would repeat its guess. These
// frames show that; an assertion cannot see it.
//
// They are a fragment with no container of their own, so the host is the row's
// line and nothing else, the frame `worklist.dispositions.stories.tsx` gives the
// judgements they share it with. The DOMAIN is the row's id: an open question is
// identified by the domain itself, the disposition row's own id never reaching a
// client.

const UNDECIDED: WorklistItem = {
  occurred_precision: "instant",
  id: "mckinsey.com",
  source: "domain_question",
  category: "decisions",
  level: 6,
  consequence: "data_drifts",
  title: "mckinsey.com",
  detail:
    "Nothing on the site named a company, and the sender's name did not explain the domain.",
  because: [],
  actions: ["keep", "discard"],
};

const KEEP = `POST /capture/domain-questions/${UNDECIDED.id}/keep`;
const DISCARD = `POST /capture/domain-questions/${UNDECIDED.id}/discard`;

// Both writes, answered at once. They go to two stores under two gates — keeping
// creates the company the triage withheld, discarding writes one seat's own
// capture exclusion — so a frame routing one proves nothing about the other.
const ME = {
  "GET /me": () =>
    jsonResponse(
      meFixture({ allow: { company: ["read", "update", "create"] } }),
    ),
};

function settleAtOnce() {
  installFetchStub({
    ...ME,
    [KEEP]: () => jsonResponse(null, 202),
    [DISCARD]: () => jsonResponse(null, 202),
  });
}

function neverSettles() {
  installFetchStub({
    ...ME,
    [KEEP]: () => new Promise<Response>(() => {}),
  });
}

function refused() {
  installFetchStub({
    ...ME,
    [KEEP]: () =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/conflict",
          title: "Conflict",
          status: 409,
          code: "conflict",
          detail: "A company already holds that domain.",
        },
        409,
      ),
  });
}

const meta: Meta<typeof DomainQuestionAnswer> = {
  title: "Records/Worklist/Domain question verbs",
  component: DomainQuestionAnswer,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof DomainQuestionAnswer>;

function line(item: WorklistItem, stub: () => void) {
  return () => {
    stub();
    return (
      <StoryProviders>
        <ToastProvider>
          <Panel title="Unreviewed domain">
            <PanelBody>
              <p className="t-caption">{item.detail}</p>
              <div className="worklist-row-acts">
                <DomainQuestionAnswer item={item} />
              </div>
            </PanelBody>
          </Panel>
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  };
}

const pressKeep = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Create company" }),
  );
};

/** Both answers, neither promoted: the pair a reader meets. */
export const TwoAnswersOfEqualWeight: Story = {
  render: line(UNDECIDED, settleAtOnce),
};

/**
 * EACH BUTTON ASKS WHETHER THE ROW OFFERS IT, rather than the pair being drawn
 * because the source is this one. A reader who may exclude the domain for
 * themselves and may not create a company is a real case, and a control drawn
 * past the server's offer is a button that 403s.
 */
export const OnlyTheAnswerTheSeatMayGive: Story = {
  render: line({ ...UNDECIDED, actions: ["discard"] }, settleAtOnce),
};

/**
 * One answer being written, and BOTH standing down. They are two answers to one
 * question, so a colleague who presses both has filed a company and an exclusion
 * for the same domain. The verb pressed is the busy one; the other is merely
 * disabled, because it started nothing.
 */
export const AnAnswerBeingWritten: Story = {
  render: line(UNDECIDED, neverSettles),
  play: pressKeep,
};

/**
 * A refused answer says so, in the product's own words rather than the server's.
 * The row is unchanged and both verbs come back live: the question is still
 * open, and the reader's next press is the retry.
 */
export const TheCompanyCouldNotBeCreated: Story = {
  render: line(UNDECIDED, refused),
  play: pressKeep,
};

const VENDOR: WorklistItem = {
  ...UNDECIDED,
  id: "company-a.example",
  title: "company-a.example",
  domain_reason_code: "vendor_verdict",
  domain_note: "Classified as a mail or hosting vendor: sells managed hosting.",
  actions: ["keep", "keep_as_vendor", "discard"],
};

/**
 * A site that reads as a mail or hosting vendor. The reader creates the
 * company for the staff writing from it, or keeps the vendor verdict. There is
 * no re-read, which would ask the same front page again.
 */
export const AVendorQuestionOffersCreateOrKeepAsVendor: Story = {
  render: line(VENDOR, () =>
    installFetchStub({
      ...ME,
      [`POST /capture/domain-questions/${VENDOR.id}/keep`]: () =>
        jsonResponse(null, 202),
      [`POST /capture/domain-questions/${VENDOR.id}/keep-vendor`]: () =>
        new Response(null, { status: 204 }),
      [`POST /capture/domain-questions/${VENDOR.id}/discard`]: () =>
        jsonResponse(null, 202),
    }),
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Keep as vendor" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Create company" }),
    ).toBeVisible();
  },
};

const ALEX = {
  id: "01a1a000-0000-7000-8000-00000000a1e7",
  name: "Alex Example",
};

const REDIRECTED: WorklistItem = {
  ...UNDECIDED,
  id: "alias.example",
  title: "alias.example",
  domain_reason_code: "forwarded_identity",
  // What the server sends for a redirected site: never keep.
  actions: ["discard", "reopen"],
  domain_context: {
    contacts: [ALEX],
    contact_count: 2,
    hidden_senders: false,
    linked_contact_count: 2,
    sender_address: "alex@alias.example",
    suggested_company: {
      id: "01a1a000-0000-7000-8000-0000000000c0",
      name: "Example Company",
      basis: "redirect_target",
    },
  },
};

// The company grant decides Confirm and Choose, so every frame below grants it.
function withCompanyGrant(routes: Record<string, () => Response>) {
  return () =>
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({ allow: { company: ["read", "update", "create"] } }),
        ),
      ...routes,
    });
}

/**
 * A redirected site with one company the evidence supports. The reader
 * confirms it or chooses another; nothing is created from the redirect.
 */
export const ARedirectWithASuggestedCompany: Story = {
  render: line(
    REDIRECTED,
    withCompanyGrant({
      [`POST /capture/domain-questions/${REDIRECTED.id}/keep`]: () =>
        jsonResponse({}, 200),
    }),
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("button", { name: "Confirm company" }),
    ).toBeVisible();
    await expect(
      canvas.getByText(
        /Confirming links alias.example to Example Company and gives 2 contacts/,
      ),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Create company" }),
    ).not.toBeInTheDocument();
  },
};
export const ARedirectWithASuggestedCompanyDark: Story = {
  ...ARedirectWithASuggestedCompany,
  globals: { theme: "dark" },
};

/** No company qualifies: the reader chooses one or creates one. */
export const NoSuggestion: Story = {
  render: line(
    {
      ...UNDECIDED,
      domain_reason_code: "unevidenced",
      actions: ["keep", "discard", "reopen"],
      domain_context: {
        contacts: [ALEX],
        contact_count: 1,
        hidden_senders: false,
        linked_contact_count: 2,
        site_read: "unread",
      },
    },
    withCompanyGrant({}),
  ),
};
export const NoSuggestionDark: Story = {
  ...NoSuggestion,
  globals: { theme: "dark" },
};

/** The vendor question gets the same choose path as every other question. */
export const AVendorQuestionCanChooseAnother: Story = {
  render: line(VENDOR, withCompanyGrant({})),
  globals: { theme: "dark" },
};
