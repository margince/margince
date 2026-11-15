/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { companyDocumentsKey } from "../lib/attachmentkeys";
import { useDedupeDisposition } from "../lib/dedupe.queries";
import { contactDocuments, PROBED_CONTACT_ID } from "./contactdocuments";

// A duplicate pair merged from the Worklist or a contact case refiles the
// merged-away record's files under the survivor.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function setup(onMerge: () => void) {
  const docs = contactDocuments(1);
  const contactReads = { count: 0 };
  vi.stubGlobal("fetch", async (request: Request) => {
    const contactList = docs.answer(request.method, new URL(request.url));
    if (contactList) {
      contactReads.count += 1;
      return contactList;
    }
    if (request.url.endsWith("/disposition")) {
      onMerge();
      return new Response(null, { status: 204 });
    }
    return Response.json({ data: [], page: { has_more: false } });
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  render(docs.strip, { wrapper });
  const decide = renderHook(() => useDedupeDisposition(), { wrapper });
  return { client, docs, contactReads, decide: decide.result };
}

it("counts the merged-away contact's files on the survivor's Documents tab", async () => {
  const { docs, decide } = setup(() => docs.becomes(2));
  await docs.shows(1);
  await act(() =>
    decide.current.mutateAsync({
      id: "pair-1",
      disposition: "merge",
      winnerId: PROBED_CONTACT_ID,
      contactIds: [PROBED_CONTACT_ID, "c-merged"],
    }),
  );
  await docs.shows(2);
});

it("refreshes both companies' libraries after a company pair merges, and no contact's", async () => {
  const { client, docs, contactReads, decide } = setup(() => {});
  await docs.shows(1);
  for (const id of ["co-kept", "co-merged", "co-other"]) {
    client.setQueryData(companyDocumentsKey(id), { data: [] });
  }
  await act(() =>
    decide.current.mutateAsync({
      id: "pair-2",
      disposition: "merge",
      winnerId: "co-kept",
      companyIds: ["co-kept", "co-merged"],
    }),
  );
  const stale = (id: string) =>
    client.getQueryState(companyDocumentsKey(id))?.isInvalidated;
  expect([stale("co-kept"), stale("co-merged"), stale("co-other")]).toEqual([
    true,
    true,
    false,
  ]);
  expect(contactReads.count).toBe(1);
});
