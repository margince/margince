/** @vitest-environment happy-dom */
import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { WhichDealChoice } from "../components/whichdealchoice";
import { ContactDealsTab } from "../features/contacts/contactdeals";
import { cancellationClient, type HeldRead, holdReads } from "./cancellation";

let held: HeldRead[] = [];
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const read of held) read.answer({});
  });
  vi.restoreAllMocks();
});

const contact: components["schemas"]["Contact360"] = {
  as_of: "2026-09-01T00:00:00Z",
  sections_omitted: [],
  contact: {
    id: "c-1",
    full_name: "Contact",
    source: "manual",
    captured_by: "human:sample",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  },
  deal_roles: {
    data: [{ relationship_id: "r-1", deal_id: "d-1", role: "champion" }],
    page: { has_more: false },
  },
};
const question: components["schemas"]["Activity"] = {
  occurred_precision: "instant",
  offers_email: false,
  id: "task-1",
  kind: "task",
  subject: "Choose a deal",
  occurred_at: "2026-09-01T00:00:00Z",
  is_done: false,
  source: "which_deal",
  source_system: "which_deal",
  source_activity_id: "mail-1",
  captured_by: "human:sample",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
  raw: { offered_deal_ids: ["d-1"] },
};

it.each([
  { name: "contact deal cards", screen: <ContactDealsTab view={contact} /> },
  { name: "mail deal choices", screen: <WhichDealChoice task={question} /> },
])("aborts the sibling deal reader used by $name", async ({ screen }) => {
  held = holdReads((url) => url.pathname === "/v1/deals/d-1");
  const { client, wrapper } = cancellationClient();
  const view = render(screen, { wrapper });
  await waitFor(() => expect(held).toHaveLength(1));
  view.unmount();
  expect(held[0]?.request.signal.aborted).toBe(true);
  client.clear();
});

it("aborts the company record reader behind a contact's deal card", async () => {
  held = holdReads((url) => url.pathname === "/v1/companies/company");
  const { client, wrapper } = cancellationClient();
  client.setQueryData(["deal", "d-1"], {
    id: "d-1",
    name: "Deal",
    company_id: "company",
    status: "open",
    amount_minor: 0,
    currency: "EUR",
    stage_id: "stage",
    pipeline_id: "pipeline",
    source: "manual",
    captured_by: "human:sample",
    version: 1,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  });
  const view = render(<ContactDealsTab view={contact} />, { wrapper });
  await waitFor(() => expect(held).toHaveLength(1));
  view.unmount();
  expect(held[0]?.request.signal.aborted).toBe(true);
  client.clear();
});
