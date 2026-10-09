// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel, PanelRow } from "../design-system/panel";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { TagSuggestionDecision } from "./worklist.tagsuggestion";

// A tag suggestion, decided on its Worklist row: the word, the mail and notes
// that raised it, and the follow-up offer once it is accepted.

type WorklistItem = components["schemas"]["WorklistItem"];

const SUGGESTION = "01a0e9f1-0000-7000-8000-0000000000d1";
const CONTACT = "01a0e9f1-0000-7000-8000-0000000000c1";
const COMPANY = "01a0e9f1-0000-7000-8000-0000000000a1";

const row: WorklistItem = {
  id: SUGGESTION,
  source: "tag_suggestion",
  category: "decisions",
  level: 6,
  consequence: "data_drifts",
  because: [],
  actions: ["decide", "dismiss", "open"],
  subject: { type: "contact", id: CONTACT, label: "Anna Weber" },
};

const suggestion = {
  id: SUGGESTION,
  state: "open",
  tag: { tag_id: "t-1", name: "Product X", color: "teal" },
  entity_type: "contact",
  entity_id: CONTACT,
  entity_name: "Anna Weber",
  created_at: "2026-10-08T09:00:00Z",
  evidence: [
    {
      activity_id: "01a0e9f1-0000-7000-8000-0000000000e1",
      kind: "email",
      subject: "Pricing for Product X",
      occurred_at: "2026-10-07T14:00:00Z",
    },
    {
      activity_id: "01a0e9f1-0000-7000-8000-0000000000e2",
      kind: "meeting",
      subject: "Intro call",
      occurred_at: "2026-10-02T10:00:00Z",
    },
  ],
};

function Served() {
  installFetchStub({
    "GET /me": meRoute({ company: ["update"], contact: ["update"] } as never),
    [`GET /tag-suggestions/${SUGGESTION}`]: () => jsonResponse(suggestion),
    [`POST /tag-suggestions/${SUGGESTION}/accept`]: () =>
      jsonResponse({ ...suggestion, state: "accepted" }),
    [`GET /contacts/${CONTACT}`]: () =>
      jsonResponse({
        id: CONTACT,
        full_name: "Anna Weber",
        employer: { company_id: COMPANY, company_name: "Demo GmbH" },
      }),
    [`GET /companies/${COMPANY}`]: () =>
      jsonResponse({ id: COMPANY, name: "Demo GmbH", writable: true }),
    [`GET /records/company/${COMPANY}/tags`]: () =>
      jsonResponse({ data: [], withheld: false }),
  });
  return (
    <StoryProviders>
      <ToastProvider>
        <Panel>
          <PanelRow>
            <TagSuggestionDecision item={row} />
          </PanelRow>
        </Panel>
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Worklist/Tag suggestion",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** The suggested word with the mail and the meeting that raised it. */
export const Open: Story = {
  render: () => <Served />,
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(/Pricing for Product X/),
    ).toBeInTheDocument();
  },
};

/** Accepted on a contact: the tag is offered to their company next. */
export const AcceptedOffersTheCompany: Story = {
  render: () => <Served />,
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", { name: "Add tag" }),
    );
    await expect(
      await within(canvasElement).findByRole("button", {
        name: "Tag Demo GmbH",
      }),
    ).toBeInTheDocument();
  },
};
