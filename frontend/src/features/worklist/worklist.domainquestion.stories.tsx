// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../../design-system/panel";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DomainQuestionAnswer } from "./worklist.domainquestion";
import type { WorklistItem } from "./worklist.queries";

// A company question, under the row that asked it. Each frame is one shape the
// question takes on the wire. The suggestion shows only when the server names
// one, and each button only when the row offers its verb.

const ALEX = {
  id: "00000000-0000-4000-8000-0000000000a1",
  name: "Alex Example",
};

const UNREAD = {
  contacts: [ALEX],
  contact_count: 1,
  hidden_senders: false,
  sender_address: "alex@mckinsey.com",
  site_unread: true,
};

const REDIRECTED = {
  contacts: [ALEX],
  contact_count: 2,
  hidden_senders: false,
  site_unread: false,
};

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
  actions: ["keep", "discard", "reopen"],
  domain_reason_code: "unevidenced",
  domain_context: UNREAD,
};

const KEEP = `POST /capture/domain-questions/${UNDECIDED.id}/keep`;
const DISCARD = `POST /capture/domain-questions/${UNDECIDED.id}/discard`;

// Both writes, answered at once. They go to two stores under two gates — keeping
// creates the company the triage withheld, discarding writes one seat's own
// capture exclusion — so a frame routing one proves nothing about the other.
function settleAtOnce() {
  installFetchStub({
    [KEEP]: () => jsonResponse(null, 202),
    [DISCARD]: () => jsonResponse(null, 202),
  });
}

function neverSettles() {
  installFetchStub({ [KEEP]: () => new Promise<Response>(() => {}) });
}

function refused() {
  installFetchStub({
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
  title: "Records/Worklist/Company question",
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
          <Panel title={item.title}>
            <PanelBody>
              <p className="t-caption">{item.detail}</p>
              <DomainQuestionAnswer item={item} />
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

/** No company to suggest: the site could not be read. Choose or create. */
export const NoSuggestionAfterAnUnreadSite: Story = {
  render: line(UNDECIDED, settleAtOnce),
};

/** A site read in vain says so, apart from one that could not be read. */
export const NoSuggestionAfterASiteReadInVain: Story = {
  render: line(
    {
      ...UNDECIDED,
      domain_context: { ...UNREAD, site_unread: false },
    },
    settleAtOnce,
  ),
};

/** Several senders: the first is named, the rest are counted. */
export const SeveralContacts: Story = {
  render: line(
    {
      ...UNDECIDED,
      domain_context: {
        ...UNREAD,
        contacts: [
          ALEX,
          { id: "00000000-0000-4000-8000-0000000000a2", name: "Bea Example" },
        ],
        contact_count: 4,
      },
    },
    settleAtOnce,
  ),
};

/** The sender is a contact the reader may not open: the address stands in. */
export const AHiddenSender: Story = {
  render: line(
    {
      ...UNDECIDED,
      domain_context: {
        contacts: [],
        contact_count: 0,
        hidden_senders: true,
        sender_address: "someone@mckinsey.com",
        site_unread: true,
      },
    },
    settleAtOnce,
  ),
};

/**
 * A reader who may exclude the domain for themselves and may not answer it
 * otherwise sees only that verb.
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
    await expect(
      canvas.getByRole("button", { name: "Choose company" }),
    ).toBeVisible();
  },
};

const FORWARDED: WorklistItem = {
  ...UNDECIDED,
  id: "alias.example",
  title: "alias.example",
  domain_reason_code: "forwarded_identity",
  actions: ["discard", "reopen"],
  domain_context: {
    ...REDIRECTED,
    suggested_company: {
      id: "00000000-0000-4000-8000-000000000001",
      name: "Example Company",
      basis: "redirect_target",
    },
  },
};

/**
 * A redirected site whose target an existing company owns. The suggestion
 * links nothing until the reader confirms, and the caption says Confirm moves
 * the whole domain. A redirected site offers no Create.
 */
export const ARedirectSuggestsACompany: Story = {
  render: line(FORWARDED, () =>
    installFetchStub({
      [`POST /capture/domain-questions/${FORWARDED.id}/keep`]: () =>
        jsonResponse(null, 202),
    }),
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Confirm company" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Choose another" }),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Create company" }),
    ).not.toBeInTheDocument();
  },
};

/** A redirected site with no company to suggest: choose, or stop capturing. */
export const ARedirectWithoutASuggestion: Story = {
  render: line(
    {
      ...FORWARDED,
      domain_context: REDIRECTED,
    },
    settleAtOnce,
  ),
};

export const ARedirectSuggestsACompanyDark: Story = {
  ...ARedirectSuggestsACompany,
  globals: { theme: "dark" },
};
export const NoSuggestionAfterAnUnreadSiteDark: Story = {
  ...NoSuggestionAfterAnUnreadSite,
  globals: { theme: "dark" },
};
