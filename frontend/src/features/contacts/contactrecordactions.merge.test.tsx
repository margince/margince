/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import {
  contactDocuments,
  PROBED_CONTACT_ID,
} from "../../testing/contactdocuments";
import { meFixture } from "../../testing/mefixture";
import { ContactRecordActions } from "./contactrecordactions";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const SOURCE = {
  id: "c-merged",
  full_name: "Dana B.",
  source: "manual",
  captured_by: "human:u1",
  version: 2,
  created_at: "2026-10-01T10:00:00Z",
  updated_at: "2026-10-01T10:00:00Z",
} satisfies components["schemas"]["Contact"];

function mountMerging() {
  const docs = contactDocuments(1);
  vi.stubGlobal("fetch", async (request: Request) => {
    const url = new URL(request.url);
    const contactList = docs.answer(request.method, url);
    if (contactList) return contactList;
    const path = url.pathname.replace(/^\/v1/, "");
    if (path === "/me") {
      return Response.json(
        meFixture({ allow: { contact: ["read", "update"] } }),
      );
    }
    if (path === "/contacts" && request.method === "GET") {
      return Response.json({
        data: [
          {
            ...SOURCE,
            id: PROBED_CONTACT_ID,
            full_name: "Dana Buyer",
            writable: true,
          },
        ],
        page: { has_more: false },
      });
    }
    if (path === `/contacts/${SOURCE.id}/merge`) {
      docs.becomes(2);
      return Response.json({ ...SOURCE, id: PROBED_CONTACT_ID });
    }
    return Response.json({ data: [], page: { has_more: false } });
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        {docs.strip}
        <ContactRecordActions contact={SOURCE} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return docs;
}

async function mergeIntoDanaBuyer() {
  const user = userEvent.setup();
  await user.click(await screen.findByTestId("merge-record"));
  await user.type(screen.getByRole("searchbox"), "Dana");
  await user.click(await screen.findByRole("button", { name: /Dana Buyer/ }));
  await user.click(screen.getByTestId("merge-confirm"));
}

it("counts the merged-away contact's files on the survivor's Documents tab", async () => {
  const docs = mountMerging();
  await docs.shows(1);
  await mergeIntoDanaBuyer();
  await docs.shows(2);
});

it("puts the survivor in the merged-away contact's place, so Back cannot reopen it", async () => {
  globalThis.history.replaceState(null, "", `#/contacts/${SOURCE.id}`);
  const entries = globalThis.history.length;
  mountMerging();
  await mergeIntoDanaBuyer();
  await waitFor(() =>
    expect(globalThis.location.hash).toBe(`#/contacts/${PROBED_CONTACT_ID}`),
  );
  expect(globalThis.history.length).toBe(entries);
});
