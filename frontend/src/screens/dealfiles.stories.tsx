// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { DealFiles, dealDocumentsKey } from "./dealfiles";
import { installFetchStub, jsonResponse } from "./story-utils";

// The deal's Files area with both kinds of row — an upload and a captured
// email attachment — and the empty state, so each reads right without a
// mailbox on a running stack.

const meta: Meta<typeof DealFiles> = {
  title: "Records/Deal/Files",
  component: DealFiles,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DealFiles>;

const UPLOAD = {
  hidden: false,
  attachment: {
    id: "att-up",
    entity_type: "deal",
    entity_id: "deal-1",
    filename: "Acme-pricing-v3.pdf",
    title: "Pricing proposal",
    category: "offer",
    source: "upload",
    captured_by: "human:u1",
    created_at: "2026-08-20T09:00:00Z",
  },
};

const CAPTURED = {
  hidden: false,
  attachment: {
    id: "att-mail",
    entity_type: "activity",
    entity_id: "act-1",
    filename: "MSA-redline.docx",
    category: "email_attachment",
    source: "gmail",
    captured_by: "human:u1",
    created_at: "2026-08-21T09:00:00Z",
  },
  origin: {
    activity_id: "act-1",
    kind: "email",
    subject: "Re: MSA",
    occurred_at: "2026-08-21T08:55:00Z",
    counterparty_email: "laura@buyer.example",
  },
};

// Both the seeded cache AND the routes are answered, and both are load-bearing:
// data written with setQueryData is stale the moment it lands, so the mount
// fires a background refetch of each key that went to the real network and came
// back 404. In jsdom nothing noticed; in the render gate's browser that 404 is a
// console error and the story fails on it.
function Served({
  docs,
  children,
}: Readonly<{ docs: unknown[]; children: ReactNode }>) {
  const documents = { data: docs, page: {} };
  const session = {
    user: { id: "u1" },
    authorization: {
      seat_type: "full",
      objects: {
        deal: { create: true, read: true, update: true, delete: true },
      },
    },
  };
  installFetchStub({
    "GET /deals/deal-1/documents": () => jsonResponse(documents),
    "GET /me": () => jsonResponse(session),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(dealDocumentsKey("deal-1", false), documents);
  client.setQueryData(["me"], session);
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

/** An upload beside an emailed file: one can be deleted, the other hidden. */
export const Mixed: Story = {
  render: () => (
    <Served docs={[CAPTURED, UPLOAD]}>
      <DealFiles dealId="deal-1" />
    </Served>
  ),
};

/** A deal with nothing on it yet. */
export const Empty: Story = {
  render: () => (
    <Served docs={[]}>
      <DealFiles dealId="deal-1" />
    </Served>
  ),
};
