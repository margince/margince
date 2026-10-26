/** @vitest-environment happy-dom */

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { viewerZone } from "../format/timezone";
import { RecordZoneProvider } from "../lib/recordzone";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { renderWithQueryClient } from "../testing/queryrender";
import { jsonResponse } from "../testing/story-utils";
import { LogActivityForm } from "./logactivity";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

// One request the page sent, and how it ended: a status, or "dropped" for a
// connection that failed with no answer.
type Call = {
  key: string;
  form: FormData | null;
  outcome?: number | "dropped";
};

const CREATED_ID = "act-kickoff";

// One installation limit, small enough that a test file can pass it.
const MAX_UPLOAD_BYTES = 1_000_000;

async function sha256Hex(file: File): Promise<string> {
  const digest = await crypto.subtle.digest(
    "SHA-256",
    await file.arrayBuffer(),
  );
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}

type Behaviour = Readonly<{
  grants?: GrantSpec;
  // Filenames the server refuses, as a 422 problem.
  refuse?: ReadonlySet<string>;
  // Filenames whose upload loses its answer; `stored` says whether the server
  // kept the bytes before the connection dropped.
  drop?: ReadonlyMap<string, { stored: boolean }>;
  // The re-read of the activity's files fails too.
  listFails?: boolean;
  // Held until the test releases it.
  holdUploads?: Promise<void>;
}>;

function stubApi(behaviour: Behaviour = {}): Call[] {
  const calls: Call[] = [];
  const stored: { filename: string; checksum: string }[] = [];
  const grants = behaviour.grants ?? { activity: ["create", "update"] };
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request ? request.url : String(input), "https://t");
    const method = request?.method ?? init?.method ?? "GET";
    const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
    const form = init?.body instanceof FormData ? init.body : null;
    const call: Call = { key, form };
    calls.push(call);
    const answer = (response: Response) => {
      call.outcome = response.status;
      return response;
    };
    if (key === "GET /me") return jsonResponse(meFixture({ allow: grants }));
    if (key === "GET /installation/settings") {
      return jsonResponse({ max_upload_bytes: MAX_UPLOAD_BYTES });
    }
    if (key === "POST /activities") {
      return jsonResponse({ id: CREATED_ID, kind: "meeting" }, 201);
    }
    if (key === "GET /attachments") {
      if (behaviour.listFails) throw new TypeError("Failed to fetch");
      return jsonResponse({
        data: stored.map((file, index) => ({ id: `att-${index}`, ...file })),
        page: { has_more: false, next_cursor: null },
      });
    }
    if (key === "POST /attachments") {
      await behaviour.holdUploads;
      const file = form?.get("file");
      if (!(file instanceof File)) throw new Error("upload carried no file");
      if (behaviour.refuse?.has(file.name)) {
        return answer(
          jsonResponse(
            {
              type: "about:blank",
              title: "Unprocessable",
              status: 422,
              detail: "This file type is not accepted.",
            },
            422,
          ),
        );
      }
      const dropped = behaviour.drop?.get(file.name);
      if (!dropped || dropped.stored) {
        stored.push({ filename: file.name, checksum: await sha256Hex(file) });
      }
      if (dropped) {
        call.outcome = "dropped";
        throw new TypeError("Failed to fetch");
      }
      return answer(jsonResponse({ id: `att-${stored.length}` }, 201));
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

const pdf = (name: string, content = "%PDF-1.4") =>
  new File([content], name, { type: "application/pdf" });

function sentFile(call: Call): File {
  const file = call.form?.get("file");
  if (!(file instanceof File)) throw new Error("the upload carried no file");
  return file;
}

const uploads = (calls: Call[]) =>
  calls.filter((call) => call.key === "POST /attachments");

const uploaded = (calls: Call[]) =>
  uploads(calls).map((call) => [sentFile(call).name, call.outcome]);

const created = (calls: Call[]) =>
  calls.some((call) => call.key === "POST /activities");

async function logWith(files: File[]) {
  const user = userEvent.setup();
  fireEvent.change(screen.getByRole("textbox", { name: "Subject" }), {
    target: { value: "Kickoff with Brandt IT" },
  });
  await user.upload(await screen.findByLabelText("Attach files"), files);
  await user.click(screen.getByRole("button", { name: "Log" }));
  return user;
}

it("uploads every picked file against the activity the server created", async () => {
  const calls = stubApi();
  renderForm();
  await logWith([pdf("price-list.pdf"), pdf("agenda.pdf")]);
  await waitFor(() => expect(uploads(calls)).toHaveLength(2));
  const createdAt = calls.findIndex((call) => call.key === "POST /activities");
  const firstUpload = calls.findIndex(
    (call) => call.key === "POST /attachments",
  );
  expect(createdAt).toBeGreaterThanOrEqual(0);
  expect(firstUpload).toBeGreaterThan(createdAt);
  expect(
    uploads(calls).map((call) => [
      call.form?.get("entity_type"),
      call.form?.get("entity_id"),
      sentFile(call).name,
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
  expect(uploaded(calls)).toEqual([
    ["macro.pdf", 422],
    ["agenda.pdf", 201],
  ]);
  expect(screen.queryByText(/agenda\.pdf was not attached/)).toBeNull();
});

it("reports nothing for an unanswered upload the server did keep", async () => {
  const calls = stubApi({
    drop: new Map([["price-list.pdf", { stored: true }]]),
  });
  renderForm();
  await logWith([pdf("price-list.pdf")]);
  await waitFor(() =>
    expect(calls.some((call) => call.key === "GET /attachments")).toBe(true),
  );
  expect(uploaded(calls)).toEqual([["price-list.pdf", "dropped"]]);
  // The form empties after the report, so an empty subject means it ran.
  await waitFor(() =>
    expect(
      (screen.getByRole("textbox", { name: "Subject" }) as HTMLInputElement)
        .value,
    ).toBe(""),
  );
  expect(screen.queryByText(/price-list\.pdf/)).toBeNull();
});

it("reports an unanswered upload as not attached once the re-read finds it missing", async () => {
  stubApi({ drop: new Map([["price-list.pdf", { stored: false }]]) });
  renderForm();
  await logWith([pdf("price-list.pdf")]);
  expect(
    await screen.findByText(/price-list\.pdf was not attached/),
  ).toBeTruthy();
});

it("says the outcome is unknown when the re-read fails too", async () => {
  stubApi({
    drop: new Map([["price-list.pdf", { stored: true }]]),
    listFails: true,
  });
  renderForm();
  await logWith([pdf("price-list.pdf")]);
  expect(
    await screen.findByText(
      "The activity is logged, but the upload of price-list.pdf did not finish. Check the activity’s files before you add it again.",
    ),
  ).toBeTruthy();
});

it("takes no new file while the log is out", async () => {
  let release = () => {};
  const holdUploads = new Promise<void>((resolve) => {
    release = resolve;
  });
  const calls = stubApi({ holdUploads });
  renderForm();
  await logWith([pdf("price-list.pdf")]);
  await waitFor(() => expect(uploads(calls)).toHaveLength(1));
  expect(screen.getByLabelText("Attach files").hasAttribute("disabled")).toBe(
    true,
  );
  release();
  await waitFor(() =>
    expect(screen.getByLabelText("Attach files").hasAttribute("disabled")).toBe(
      false,
    ),
  );
  expect(uploaded(calls)).toEqual([["price-list.pdf", 201]]);
});

it("keeps two different files that share a name, size and date", async () => {
  const calls = stubApi();
  renderForm();
  const lastModified = Date.UTC(2026, 6, 6);
  await logWith([
    new File(["AAAA"], "scan.pdf", { lastModified }),
    new File(["BBBB"], "scan.pdf", { lastModified }),
  ]);
  await waitFor(() => expect(uploads(calls)).toHaveLength(2));
  const sent = await Promise.all(
    uploads(calls).map((call) => sentFile(call).text()),
  );
  expect(sent).toEqual(["AAAA", "BBBB"]);
});

it("refuses a file over the installation limit before anything is logged", async () => {
  const calls = stubApi();
  const user = userEvent.setup();
  renderForm();
  const input = await screen.findByLabelText("Attach files");
  // The hint names the limit once the installation has stated it.
  await screen.findByText("Up to 1 MB.");
  await user.upload(
    input,
    new File([new Uint8Array(MAX_UPLOAD_BYTES + 1)], "scan.pdf"),
  );
  expect(
    screen.getByText(
      "The file exceeds 1 MB, the limit on this installation. Choose a smaller file.",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /Remove scan\.pdf/ })).toBeNull();
  expect(created(calls)).toBe(false);
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
  await waitFor(() => expect(created(calls)).toBe(true));
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
