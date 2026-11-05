// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import { SearchHit } from "./searchhit";
import "./search.css";

type EmailSummary = components["schemas"]["EmailSummary"];
type SearchResult = components["schemas"]["SearchResult"];

function mailHit(over: Partial<EmailSummary>): SearchResult {
  return {
    type: "activity",
    id: "m-1",
    title: "Acme renewal terms",
    score: 8,
    trust_tier: "authoritative",
    email_summary: {
      activity_id: "m-1",
      occurred_at: "2026-09-03T09:15:00Z",
      occurred_precision: "instant",
      version: 1,
      subject: "Acme renewal terms",
      preview: "Acme asked again about the renewal terms.",
      counterparty: "Dana Buyer",
      direction: "inbound",
      display_status: "team",
      move: "needs_reply",
      attachment_count: 0,
      ...over,
    },
  };
}

function renderHit(hit: SearchResult) {
  return (
    <StoryProviders>
      <ul className="search-hits">
        <SearchHit hit={hit} onOpen={() => undefined} />
      </ul>
    </StoryProviders>
  );
}

const meta = {
  title: "Records/Search hit",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;
type Story = StoryObj;

const entry = (canvasElement: HTMLElement) =>
  within(canvasElement).findByRole("button", { name: /Acme renewal terms/ });

export const MailAtAnInstant: Story = {
  render: () => renderHit(mailHit({})),
  play: async ({ canvasElement }) => {
    await expect((await entry(canvasElement)).textContent).toMatch(
      /\d{1,2}:\d{2}/,
    );
  },
};

// A message logged by its day alone names the date and no clock time.
export const MailOnlyItsDayKnown: Story = {
  render: () =>
    renderHit(
      mailHit({
        occurred_at: "2026-09-03T12:00:00Z",
        occurred_precision: "day",
      }),
    ),
  play: async ({ canvasElement }) => {
    await expect((await entry(canvasElement)).textContent).not.toMatch(
      /\d{1,2}:\d{2}/,
    );
  },
};
