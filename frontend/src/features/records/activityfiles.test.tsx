/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { renderWithQueryClient } from "../../testing/queryrender";
import { jsonResponse } from "../../testing/story-utils";
import { ActivityFiles, ActivityFilesAction } from "./activityfiles";

type Activity = components["schemas"]["Activity"];
type Attachment = components["schemas"]["Attachment"];

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

const PRICE_LIST: Attachment = {
  id: "att-price",
  entity_type: "activity",
  entity_id: "act-kickoff",
  filename: "price-list-2026.pdf",
  created_at: "2026-07-06T09:00:00Z",
  source: "upload",
  captured_by: "human:u1",
};

const KICKOFF: Activity = {
  writable: true,
  id: "act-kickoff",
  kind: "meeting",
  subject: "Kickoff with Brandt IT",
  occurred_at: "2026-07-06T09:00:00Z",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-07-06T09:00:00Z",
  updated_at: "2026-07-06T09:00:00Z",
} as Activity;

type Call = { key: string; query: URLSearchParams; form: FormData | null };

// Multipart bodies are kept as the FormData the page sent, which the shared
// stub would parse as JSON and lose.
function stubApi(
  files: Attachment[],
  grants: GrantSpec = {
    activity: ["read", "update"],
  },
  uploadAnswer: () => Response = () =>
    jsonResponse({ ...PRICE_LIST, id: "att-new" }, 201),
): Call[] {
  const calls: Call[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request ? request.url : String(input), "https://t");
    const method = request?.method ?? init?.method ?? "GET";
    const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
    const form = init?.body instanceof FormData ? init.body : null;
    calls.push({ key, query: url.searchParams, form });
    if (key === "GET /me") return jsonResponse(meFixture({ allow: grants }));
    if (key === "GET /attachments") {
      return jsonResponse({
        data: files,
        page: { has_more: false, next_cursor: null },
      });
    }
    if (key === "POST /attachments") return uploadAnswer();
    return jsonResponse({ data: [], page: { next_cursor: null } });
  }) as typeof fetch;
  return calls;
}

const listReads = (calls: Call[]) =>
  calls.filter((call) => call.key === "GET /attachments");

async function uploadThroughDialog(user: ReturnType<typeof userEvent.setup>) {
  const add = await screen.findByRole("button", { name: "Add document" });
  await waitFor(() => expect(add.hasAttribute("disabled")).toBe(false));
  await user.click(add);
  const dialog = await screen.findByRole("dialog");
  await user.upload(
    within(dialog).getByLabelText("File"),
    new File(["prices"], "price-list-2026.pdf", { type: "application/pdf" }),
  );
  await user.click(within(dialog).getByRole("button", { name: "Upload" }));
  return dialog;
}

describe("ActivityFiles", () => {
  it("lists the activity's own files, each linking to its bytes", async () => {
    const calls = stubApi([PRICE_LIST]);
    renderWithQueryClient(<ActivityFiles activityId="act-kickoff" writable />);
    const link = await screen.findByRole("link", {
      name: /price-list-2026\.pdf/,
    });
    expect(link.getAttribute("href")).toBe("/v1/attachments/att-price");
    const [read] = listReads(calls);
    expect(read?.query.get("entity_type")).toBe("activity");
    expect(read?.query.get("entity_id")).toBe("act-kickoff");
  });

  it("says so when the activity has no files", async () => {
    stubApi([]);
    renderWithQueryClient(<ActivityFiles activityId="act-kickoff" writable />);
    expect(
      await screen.findByText("No files on this activity yet."),
    ).toBeTruthy();
  });

  it("uploads a file against the activity and reads the list again", async () => {
    const calls = stubApi([]);
    const user = userEvent.setup();
    renderWithQueryClient(<ActivityFiles activityId="act-kickoff" writable />);
    await uploadThroughDialog(user);
    await waitFor(() => expect(listReads(calls)).toHaveLength(2));
    const upload = calls.find((call) => call.key === "POST /attachments");
    expect(upload?.form?.get("entity_type")).toBe("activity");
    expect(upload?.form?.get("entity_id")).toBe("act-kickoff");
  });

  it("refreshes every file list that shows the activity's files", async () => {
    stubApi([]);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ActivityFiles activityId="act-kickoff" writable />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await uploadThroughDialog(user);
    await waitFor(() =>
      expect(invalidate.mock.calls.map(([filter]) => filter?.queryKey)).toEqual(
        [
          ["attachments", "activity", "act-kickoff"],
          ["deal-documents"],
          ["deal-attachments"],
          ["companyDocuments"],
        ],
      ),
    );
  });

  it("shows the refusal when the activity is not this rep's to change", async () => {
    // A stale read can still offer the control; the server's refusal then shows.
    const calls = stubApi([], undefined, () =>
      jsonResponse(
        {
          type: "urn:margince:problem:permission_denied",
          status: 403,
          code: "permission_denied",
          title: "Forbidden",
        },
        403,
      ),
    );
    const user = userEvent.setup();
    renderWithQueryClient(<ActivityFiles activityId="act-kickoff" writable />);
    const dialog = await uploadThroughDialog(user);
    expect(await within(dialog).findByText("Upload failed")).toBeTruthy();
    expect(listReads(calls)).toHaveLength(1);
  });

  it("says why a seat that may not change activities cannot upload", async () => {
    stubApi([], { activity: ["read"] });
    renderWithQueryClient(<ActivityFiles activityId="act-kickoff" writable />);
    const reason = await screen.findByText(
      "You do not have permission to add documents to this record.",
    );
    const add = screen.getByRole("button", { name: "Add document" });
    expect(add.getAttribute("aria-describedby")).toBe(reason.id);
    expect(add.hasAttribute("disabled")).toBe(true);
  });

  it("says why a rep cannot upload to a colleague's activity", async () => {
    stubApi([]);
    renderWithQueryClient(
      <ActivityFiles activityId="act-kickoff" writable={false} />,
    );
    const reason = await screen.findByText(
      "You do not have permission to add documents to this record.",
    );
    const add = screen.getByRole("button", { name: "Add document" });
    expect(add.getAttribute("aria-describedby")).toBe(reason.id);
    expect(add.hasAttribute("disabled")).toBe(true);
  });

  it("offers no upload on an archived activity", async () => {
    stubApi([PRICE_LIST]);
    renderWithQueryClient(
      <ActivityFiles activityId="act-kickoff" writable archived />,
    );
    await screen.findByRole("link", { name: /price-list-2026\.pdf/ });
    expect(screen.queryByRole("button", { name: "Add document" })).toBeNull();
  });
});

describe("ActivityFilesAction", () => {
  it("reads the files only once the reader opens them", async () => {
    const calls = stubApi([PRICE_LIST]);
    const user = userEvent.setup();
    renderWithQueryClient(<ActivityFilesAction activity={KICKOFF} />);
    const open = screen.getByRole("button", { name: "Files" });
    expect(listReads(calls)).toHaveLength(0);
    await user.click(open);
    const dialog = await screen.findByRole("dialog", {
      name: "Files on Kickoff with Brandt IT",
    });
    expect(
      await within(dialog).findByRole("link", { name: /price-list-2026\.pdf/ }),
    ).toBeTruthy();
  });

  it("is not drawn on captured mail, whose drawer carries its files", () => {
    stubApi([]);
    renderWithQueryClient(
      <ActivityFilesAction activity={{ ...KICKOFF, kind: "email" }} />,
    );
    expect(screen.queryByRole("button", { name: "Files" })).toBeNull();
  });

  it("is not drawn on a row whose content the reader may not read", () => {
    stubApi([]);
    renderWithQueryClient(
      <ActivityFilesAction
        activity={{ ...KICKOFF, content_state: "withheld" }}
      />,
    );
    expect(screen.queryByRole("button", { name: "Files" })).toBeNull();
  });
});
