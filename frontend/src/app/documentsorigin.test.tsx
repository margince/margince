/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { CompanyDocumentsCard } from "../features/companies/companydocuments";
import { ContactFilesTab } from "../features/contacts/contactfiles";
import { DealFiles } from "../features/deals/dealfiles";
import { LocaleProvider } from "../i18n";
import { createQueryClient } from "./queryclient";

type Attachment = components["schemas"]["Attachment"];
type Deal = components["schemas"]["Deal"];

// One file that came with one mail, as each Documents list receives it.
const MAILED: Attachment = {
  id: "att-mail",
  entity_type: "activity",
  entity_id: "act-1",
  filename: "MSA-redline.docx",
  category: "email_attachment",
  source: "gmail",
  captured_by: "human:u1",
  created_at: "2026-08-21T09:00:00Z",
  origin: {
    activity_id: "act-1",
    kind: "email",
    subject: "Re: MSA",
    occurred_at: "2026-08-21T08:55:00Z",
    occurred_precision: "instant",
    sender: { address: "laura@buyer.example", display_name: "Laura Buyer" },
  },
};

const DEAL = {
  id: "deal-1",
  name: "Fleet retrofit",
  pipeline_id: "pl",
  stage_id: "s1",
  status: "open",
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-01T00:00:00Z",
} as Deal;

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

// Each list's own read answers the same file: the deal's wrapped as a
// DealDocument, the others as the attachment itself.
function stubLists() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/me")) {
        return json({
          user: { id: "u1", email: "rep@example.com", name: "Demo Rep" },
          authorization: { seat_type: "full", objects: {} },
        });
      }
      if (pathname.endsWith("/deals/deal-1/documents")) {
        return json({
          data: [{ attachment: MAILED, hidden: false }],
          page: {},
        });
      }
      if (
        pathname.endsWith("/attachments") ||
        pathname.endsWith("/companies/co-1/documents")
      ) {
        return json({ data: [MAILED], page: { has_more: false } });
      }
      return json({ data: [], page: {} });
    }),
  );
}

function show(ui: ReactNode) {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The same mail must read the same on every Documents list. A list that words
// it from another field tells a reader two things about one file.
describe("a file that came with a mail", () => {
  it.each([
    ["the contact's Documents tab", <ContactFilesTab contactId="p-1" />],
    ["the company's Documents card", <CompanyDocumentsCard companyId="co-1" />],
    ["the deal's Files list", <DealFiles deal={DEAL} />],
  ])("names its mail by sender and subject on %s", async (_surface, ui) => {
    stubLists();
    show(ui);
    expect(
      await screen.findByText("From Laura Buyer · Re: MSA"),
    ).toBeInTheDocument();
  });
});
