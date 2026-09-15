// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { DealMailAside } from "./dealmailaside";
import { installFetchStub, jsonResponse } from "./story-utils";

// The flyout under a board card's mail line, drawn on its own so the three
// readings it has can be judged without a board around them: an exchange the
// reader may read, one with a message held from them, and a deal whose mail
// this reader may know nothing about.

const meta: Meta<typeof DealMailAside> = {
  title: "Records/Deal/Mail flyout",
  component: DealMailAside,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DealMailAside>;

const DEAL = "deal-1";
const DAY_MS = 24 * 60 * 60 * 1000;

// Relative to the moment the story renders rather than to a fixed date, so
// the words say "10 d ago" on every day the canvas is opened.
function daysAgo(days: number): string {
  return new Date(Date.now() - days * DAY_MS).toISOString();
}

function mail(
  id: string,
  subject: string | null,
  direction: "inbound" | "outbound",
  days: number,
  withheld = false,
) {
  return {
    id,
    kind: "email",
    subject: withheld ? null : subject,
    direction,
    occurred_at: daysAgo(days),
    content_state: withheld ? "withheld" : "available",
    source: "gmail",
    captured_by: "connector:gmail",
    created_at: daysAgo(days),
    updated_at: daysAgo(days),
  };
}

function Served({
  rows,
  children,
}: Readonly<{ rows: unknown[]; children: ReactNode }>) {
  installFetchStub({
    "GET /activities": () =>
      jsonResponse({ data: rows, page: { next_cursor: null } }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <div style={{ maxWidth: "20rem" }}>{children}</div>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

// We wrote twice and they answered once, two months back: the shape of a
// deal going quiet, read in three lines.
export const AnExchange: Story = {
  render: () => (
    <Served
      rows={[
        mail("a1", "AW: Ausbildungsoffensive Bayern", "outbound", 10),
        mail("a2", "Ausbildungsoffensive Bayern, final", "outbound", 11),
        mail("a3", "AW: RetrieverClub MVP", "inbound", 61),
      ]}
    >
      <DealMailAside dealId={DEAL} href={`#/deals/${DEAL}`} />
    </Served>
  ),
};

// The newest message is one this reader may know about but not read: cited in
// the timeline's own words for that, with nothing to press.
export const WithAHeldMessage: Story = {
  render: () => (
    <Served
      rows={[
        mail("a1", "Board pack", "inbound", 1, true),
        mail("a2", "AW: Ausbildungsoffensive Bayern", "outbound", 10),
      ]}
    >
      <DealMailAside dealId={DEAL} href={`#/deals/${DEAL}`} />
    </Served>
  ),
};

// Nothing this reader may know about, said so, with the door still offered.
export const NothingToShow: Story = {
  render: () => (
    <Served rows={[]}>
      <DealMailAside dealId={DEAL} href={`#/deals/${DEAL}`} />
    </Served>
  ),
};
