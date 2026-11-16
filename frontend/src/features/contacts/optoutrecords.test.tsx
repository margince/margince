/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { ContactConsentPanel } from "./contactconsentpanel";

// The consent panel hands an inspector the contact's opt-out records as one
// file, as the server answered them.

const downloadBytes = vi.hoisted(() => vi.fn());
vi.mock("../../lib/download", () => ({ downloadBytes }));

type Contact360 = components["schemas"]["Contact360"];
type OptOutRecords = components["schemas"]["OptOutRecords"];

const RECORDS: OptOutRecords = {
  contact_id: "p-1",
  generated_at: "2026-11-16T09:00:00Z",
  consent_events: [],
  stops: [
    {
      id: "s-1",
      kind: "marketing_objection",
      received_at: "2026-11-16T05:00:00Z",
      stopped_at: "2026-11-16T07:00:00Z",
    },
  ],
  receipts: [
    {
      message_id: "optout-ack-s1@margince.invalid",
      status: "sent",
      template_version: 4,
      queued_at: "2026-11-16T07:00:00Z",
    },
  ],
};

const VIEW: Contact360 = {
  as_of: "2026-11-16T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-06-01T08:00:00Z",
    version: 1,
    emails: [],
    phones: [],
  },
  activities: { data: [], page: { has_more: false } },
  sections_omitted: [],
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("Download opt-out records", () => {
  it("saves the server's records as a JSON file named for the contact", async () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /contacts/p-1/consent/optout-records": () => jsonResponse(RECORDS),
    });
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <ContactConsentPanel view={VIEW} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Download opt-out records" }),
    );

    await waitFor(() => expect(downloadBytes).toHaveBeenCalledTimes(1));
    const [bytes, filename, type] = downloadBytes.mock.calls[0] ?? [];
    expect(JSON.parse(String(bytes))).toEqual(RECORDS);
    expect(filename).toBe("optout-records-p-1.json");
    expect(type).toBe("application/json");
  });
});
