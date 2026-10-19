/** @vitest-environment happy-dom */

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { viewerZone } from "../../format/timezone";
import { RecordZoneProvider } from "../../lib/recordzone";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { renderWithQueryClient } from "../../testing/queryrender";
import { jsonResponse } from "../../testing/story-utils";
import { LogActivityForm } from "./logactivity";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

type Call = { key: string; form: FormData | null };

const CREATED_ID = "act-kickoff";

// One installation limit, small enough that a test file can pass it.
const MAX_UPLOAD_BYTES = 1_000_000;

function stubApi({
  grants = { activity: ["create", "update"] },
  refuse = new Set<string>(),
}: Readonly<{
  grants?: GrantSpec;
  // Filenames the server refuses, as a 422 problem.
  refuse?: ReadonlySet<string>;
}> = {}): Call[] {
  const calls: Call[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request ? request.url : String(input), "https://t");
    const method = request?.method ?? init?.method ?? "GET";
    const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
    const form = init?.body instanceof FormData ? init.body : null;
    calls.push({ key, form });
    if (key === "GET /me") return jsonResponse(meFixture({ allow: grants }));
    if (key === "GET /installation/settings") {
      return jsonResponse({ max_upload_bytes: MAX_UPLOAD_BYTES });
    }
    if (key === "POST /activities") {
      return jsonResponse({ id: CREATED_ID, kind: "meeting" }, 201);
    }
    if (key === "POST /attachments") {
      const file = form?.get("file");
      if (file instanceof File && refuse.has(file.name)) {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Unprocessable",
            status: 422,
            detail: "This file type is not accepted.",
          },
          422,
        );
      }
      return jsonResponse({ id: `att-${calls.length}` }, 201);
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  }) as typeof fetch;
  return calls;
}

function renderForm() {
  return renderWithQueryClient(
    <ToastProvider>
      <RecordZoneProvider zone={viewerZone()}>
        <LogActivityForm entityType="contact" entityId="contact-one" />
      </RecordZoneProvider>
      <ToastRegion />
    </ToastProvider>,
  );
}

const pdf = (name: string, bytes = 8) =>
  new File([new Uint8Array(bytes)], name, { type: "application/pdf" });

const uploads = (calls: Call[]) =>
  calls.filter((call) => call.key === "POST /attachments");

async function logWith(files: File[]) {
  const user = userEvent.setup();
  fireEvent.change(screen.getByRole("textbox", { name: "Subject" }), {
    target: { value: "Kickoff with Brandt IT" },
  });
  await user.upload(await screen.findByLabelText("Attach files"), files);
  await user.click(screen.getByRole("button", { name: "Log" }));
}

it("uploads every picked file against the activity the server created", async () => {
  const calls = stubApi();
  renderForm();
  await logWith([pdf("price-list.pdf"), pdf("agenda.pdf")]);
  await waitFor(() => expect(uploads(calls)).toHaveLength(2));
  const created = calls.findIndex((call) => call.key === "POST /activities");
  const firstUpload = calls.findIndex(
    (call) => call.key === "POST /attachments",
  );
  expect(created).toBeGreaterThanOrEqual(0);
  expect(firstUpload).toBeGreaterThan(created);
  expect(
    uploads(calls).map((call) => [
      call.form?.get("entity_type"),
      call.form?.get("entity_id"),
      (call.form?.get("file") as File | null)?.name,
    ]),
  ).toEqual([
    ["activity", CREATED_ID, "price-list.pdf"],
    ["activity", CREATED_ID, "agenda.pdf"],
  ]);
});

it("says which file was refused and still uploads the rest", async () => {
  const calls = stubApi({ refuse: new Set(["macro.pdf"]) });
  renderForm();
  await logWith([pdf("macro.pdf"), pdf("agenda.pdf")]);
  expect(
    await screen.findByText(
      "The activity is logged, but macro.pdf was not attached. This file type is not accepted.",
    ),
  ).toBeTruthy();
  expect(uploads(calls)).toHaveLength(2);
});

it("refuses a file over the installation limit before anything is logged", async () => {
  const calls = stubApi();
  const user = userEvent.setup();
  renderForm();
  const input = await screen.findByLabelText("Attach files");
  // The hint names the limit once the installation has stated it.
  await screen.findByText("Up to 1 MB.");
  await user.upload(input, pdf("scan.pdf", MAX_UPLOAD_BYTES + 1));
  expect(
    screen.getByText(
      "The file exceeds 1 MB, the limit on this installation. Choose a smaller file.",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /Remove scan\.pdf/ })).toBeNull();
  expect(calls.some((call) => call.key === "POST /activities")).toBe(false);
});

it("lets a picked file be taken off before logging", async () => {
  const calls = stubApi();
  const user = userEvent.setup();
  renderForm();
  await user.upload(await screen.findByLabelText("Attach files"), [
    pdf("draft.pdf"),
  ]);
  await user.click(screen.getByRole("button", { name: /Remove draft\.pdf/ }));
  fireEvent.change(screen.getByRole("textbox", { name: "Subject" }), {
    target: { value: "Kickoff with Brandt IT" },
  });
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() =>
    expect(calls.some((call) => call.key === "POST /activities")).toBe(true),
  );
  expect(uploads(calls)).toHaveLength(0);
});

it("offers no files to a seat that may not change activities", async () => {
  const calls = stubApi({ grants: { activity: ["create"] } });
  renderForm();
  await waitFor(() =>
    expect(calls.some((call) => call.key === "GET /me")).toBe(true),
  );
  await screen.findByRole("textbox", { name: "Subject" });
  expect(screen.queryByLabelText("Attach files")).toBeNull();
});
