import { waitFor } from "@testing-library/react";
import { expect } from "vitest";
import { populated } from "../features/contacts/contactpage.fixtures";
import { ContactRecordTabs } from "../features/contacts/contactpage.tabs";

// The contact page's tab strip, mounted beside a writer under test, so the test
// reads the Documents count a reader would see after the write. The server's
// list is a count the test changes when its stub performs the write.

export const PROBED_CONTACT_ID = populated.contact.id;

export function contactDocuments(initial: number) {
  let count = initial;
  const list = () =>
    Response.json({
      data: Array.from({ length: count }, (_, at) => ({
        id: `contact-file-${at}`,
        entity_type: "contact",
        entity_id: PROBED_CONTACT_ID,
        filename: `file-${at}.pdf`,
        source: "upload",
        created_at: "2026-07-06T09:00:00Z",
      })),
      page: { has_more: false, next_cursor: null },
    });
  return {
    /** The list read's answer, or undefined for any other request. */
    answer(method: string, url: URL): Response | undefined {
      const read =
        method === "GET" &&
        url.pathname.replace(/^\/v1/, "") === "/attachments" &&
        url.searchParams.get("entity_type") === "contact" &&
        url.searchParams.get("entity_id") === PROBED_CONTACT_ID;
      return read ? list() : undefined;
    },
    /** The list as the server holds it now, for a stub routed by path alone. */
    list,
    /** What the server's list holds from now on. */
    becomes(next: number) {
      count = next;
    },
    strip: (
      <ContactRecordTabs
        view={populated}
        value="overview"
        onChange={() => {}}
      />
    ),
    /** Waits for the strip to print this count. */
    shows: (expected: number) =>
      waitFor(() =>
        expect(
          document.querySelector("button[data-tab='documents']")?.textContent,
        ).toBe(`Documents, ${expected}`),
      ),
  };
}
