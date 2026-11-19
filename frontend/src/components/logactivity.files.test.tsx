/** @vitest-environment happy-dom */

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { browserZone } from "../format/timezone";
import { CompanyZoneProvider } from "../lib/companyzone";
import {
  contactDocuments,
  PROBED_CONTACT_ID,
} from "../testing/contactdocuments";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { renderWithQueryClient } from "../testing/queryrender";
import { jsonResponse } from "../testing/story-utils";
import { LogActivityForm } from "./logactivity";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.useRealTimers();
});

// One request the page sent, and how it ended: a status, or "dropped" for a
// connection that failed with no answer.
type Call = {
  key: string;
  form: FormData | null;
  outcome?: number | "dropped";
  // A JSON write's body, precondition and key, as the server would read them.
  json?: unknown;
  ifMatch?: string | null;
  text?: string;
  idempotencyKey?: string | null;
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
  // The contact page's Documents count, mounted beside the form.
  docs?: ReturnType<typeof contactDocuments>;
  // How many task completions the server answers with a 500 before one lands.
  completionFailures?: { left: number; status?: number; detail?: string };
  // How many creates the server stores and then loses the answer to.
  lostCreateAnswers?: { left: number };
  // How many creates the server refuses with a 422, storing nothing.
  refusedCreates?: { left: number };
}>;

function stubApi(behaviour: Behaviour = {}): Call[] {
  const calls: Call[] = [];
  const stored: { filename: string; checksum: string }[] = [];
  const grants = behaviour.grants ?? { activity: ["create", "update"] };
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request ? request.url : String(input), "https://t");
    const method = request?.method ?? init?.method ?? "GET";
    const contactList = behaviour.docs?.answer(method, url);
    if (contactList) return contactList;
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
      call.text = await request?.clone().text();
      call.idempotencyKey = request?.headers.get("Idempotency-Key");
      const lost = behaviour.lostCreateAnswers;
      if (lost && lost.left > 0) {
        lost.left -= 1;
        call.outcome = "dropped";
        throw new TypeError("Failed to fetch");
      }
      const refusedCreate = behaviour.refusedCreates;
      if (refusedCreate && refusedCreate.left > 0) {
        refusedCreate.left -= 1;
        return answer(
          jsonResponse(
            { type: "about:blank", title: "Unprocessable", status: 422 },
            422,
          ),
        );
      }
      return jsonResponse({ id: CREATED_ID, kind: "meeting" }, 201);
    }
    if (key === "GET /attachments") {
      if (behaviour.listFails) throw new TypeError("Failed to fetch");
      return jsonResponse({
        data: stored.map((file, index) => ({ id: `att-${index}`, ...file })),
        page: { has_more: false, next_cursor: null },
      });
    }
    if (key === "GET /activities/task-one")
      return jsonResponse({
        id: "task-one",
        kind: "task",
        is_done: false,
        version: 1,
      });
    if (key === "PATCH /activities/task-one") {
      call.json = await request?.clone().json();
      call.ifMatch = request?.headers.get("If-Match");
      const failures = behaviour.completionFailures;
      if (failures && failures.left > 0) {
        failures.left -= 1;
        const status = failures.status ?? 500;
        return answer(
          jsonResponse(
            {
              type: "about:blank",
              title: "Refused",
              status,
              detail: failures.detail,
            },
            status,
          ),
        );
      }
      return answer(
        jsonResponse({
          id: "task-one",
          kind: "task",
          is_done: true,
          version: 2,
        }),
      );
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
        behaviour.docs?.becomes(1 + stored.length);
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

function renderForm(task = false, docs?: ReturnType<typeof contactDocuments>) {
  return renderWithQueryClient(
    <ToastProvider>
      {docs?.strip}
      <CompanyZoneProvider zone={browserZone()}>
        <LogActivityForm
          entityType="contact"
          entityId={docs ? PROBED_CONTACT_ID : "contact-one"}
          sourceActivityId={task ? "task-one" : undefined}
          taskToComplete={task ? { id: "task-one", version: 1 } : undefined}
        />
      </CompanyZoneProvider>
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

const LOGGED_ONLY =
  "The activity is logged and can no longer be changed here. Log again to finish the steps that are left.";

// The create bodies in the order they were sent.
const createBodies = (calls: Call[]) =>
  calls
    .filter((call) => call.key === "POST /activities")
    .map((call) => JSON.parse(call.text ?? "{}"));

const creates = (calls: Call[]) =>
  calls.filter((call) => call.key === "POST /activities").length;

const completions = (calls: Call[]) =>
  calls.filter((call) => call.key === "PATCH /activities/task-one");

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

it("counts the logged files in the contact's Documents tab", async () => {
  const docs = contactDocuments(1);
  stubApi({ docs });
  renderForm(false, docs);
  await docs.shows(1);
  await logWith([pdf("price-list.pdf"), pdf("agenda.pdf")]);
  await docs.shows(3);
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
      "scan.pdf was not added. The file exceeds 1 MB, the limit on this installation. Choose a smaller file.",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: /Remove scan\.pdf/ })).toBeNull();
  expect(created(calls)).toBe(false);
});

it("names each dropped file the server would refuse, before anything is logged", async () => {
  const calls = stubApi();
  renderForm();
  const input = await screen.findByLabelText("Attach files");
  await screen.findByText("Up to 1 MB.");
  // A drop skips the picker's filter, so these reach the form as they are.
  fireEvent.drop(input, {
    dataTransfer: {
      files: [
        new File(["MZ"], "setup.exe", { type: "application/x-msdownload" }),
        new File([], "blank.pdf", { type: "application/pdf" }),
        pdf("agenda.pdf"),
      ],
    },
  });
  expect(
    await screen.findByText(
      "setup.exe was not added. This kind of file cannot be attached. Choose a document, image, zip archive or saved email.",
    ),
  ).toBeTruthy();
  expect(
    screen.getByText(
      "blank.pdf was not added. The file is empty, so there is nothing to add. Choose the file again.",
    ),
  ).toBeTruthy();
  expect(
    screen.getByRole("button", { name: /Remove agenda\.pdf/ }),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: /Remove setup\.exe/ }),
  ).toBeNull();
  expect(
    screen.queryByRole("button", { name: /Remove blank\.pdf/ }),
  ).toBeNull();
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

it("logs a task outcome with its file and completes the task when selected", async () => {
  const calls = stubApi();
  renderForm(true);
  await logWith([pdf("outcome.pdf")]);
  await waitFor(() => expect(completions(calls)).toHaveLength(1));
  expect(uploaded(calls)).toEqual([["outcome.pdf", 201]]);
  // The version goes in If-Match only; the update body declares no version.
  expect(completions(calls)[0]).toMatchObject({
    json: { is_done: true },
    ifMatch: "1",
  });
  expect(completions(calls)[0]?.json).toStrictEqual({ is_done: true });
});

it("says the task is still open when only its completion fails, and a retry only closes it", async () => {
  const calls = stubApi({ completionFailures: { left: 1 } });
  renderForm(true);
  const user = await logWith([pdf("outcome.pdf")]);
  expect(
    await screen.findByText(
      "The activity was logged, but the task is still open. Retry to close it.",
    ),
  ).toBeTruthy();
  expect(creates(calls)).toBe(1);
  // The logged words can no longer be edited, so no edit is lost on the retry.
  expect(screen.queryByRole("textbox", { name: "Subject" })).toBeNull();
  expect(screen.getByText(LOGGED_ONLY)).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() =>
    expect(completions(calls).map((call) => call.outcome)).toEqual([500, 200]),
  );
  expect(creates(calls)).toBe(1);
  expect(uploaded(calls)).toEqual([["outcome.pdf", 201]]);
  await waitFor(() =>
    expect(
      screen.getByRole("textbox", { name: "Subject" }).getAttribute("value"),
    ).toBe(""),
  );
});

it("keeps the task open when completion is unchecked", async () => {
  const calls = stubApi();
  renderForm(true);
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "Mark task done" }));
  await logWith([pdf("outcome.pdf")]);
  await waitFor(() => expect(uploaded(calls)).toEqual([["outcome.pdf", 201]]));
  expect(calls.some((call) => call.key === "PATCH /activities/task-one")).toBe(
    false,
  );
});

it("keeps a partially attached outcome open and retries only its missing file", async () => {
  const refused = new Set(["outcome.pdf"]);
  const calls = stubApi({ refuse: refused });
  renderForm(true);
  const user = await logWith([pdf("agenda.pdf"), pdf("outcome.pdf")]);
  await waitFor(() =>
    expect(uploaded(calls)).toEqual([
      ["agenda.pdf", 201],
      ["outcome.pdf", 422],
    ]),
  );
  expect(calls.some((call) => call.key === "PATCH /activities/task-one")).toBe(
    false,
  );
  expect(screen.getByText(LOGGED_ONLY)).toBeTruthy();
  refused.clear();
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() =>
    expect(
      calls.some((call) => call.key === "PATCH /activities/task-one"),
    ).toBe(true),
  );
  expect(uploaded(calls)).toEqual([
    ["agenda.pdf", 201],
    ["outcome.pdf", 422],
    ["outcome.pdf", 201],
  ]);
  expect(creates(calls)).toBe(1);
});

it("resends the identical create under the same key when its answer was lost", async () => {
  // Only Date is faked, so the clock can move between tries while events run.
  vi.useFakeTimers({ toFake: ["Date"] });
  const calls = stubApi({ lostCreateAnswers: { left: 1 } });
  renderForm();
  const user = userEvent.setup();
  fireEvent.change(await screen.findByRole("textbox", { name: "Subject" }), {
    target: { value: "Kickoff with Brandt IT" },
  });
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(creates(calls)).toBe(1));
  await screen.findByRole("alert");
  vi.setSystemTime(Date.now() + 60_000);
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(creates(calls)).toBe(2));
  const [lost, retry] = calls.filter((call) => call.key === "POST /activities");
  expect(retry?.idempotencyKey).toBe(lost?.idempotencyKey);
  expect(retry?.text).toBe(lost?.text);
  expect(lost?.text).toContain('"occurred_at"');
});

it("sends an edited draft as edited after the server refused the create", async () => {
  const calls = stubApi({ refusedCreates: { left: 1 } });
  renderForm();
  const user = userEvent.setup();
  const subject = await screen.findByRole("textbox", { name: "Subject" });
  fireEvent.change(subject, { target: { value: "Kickoff" } });
  await user.click(screen.getByRole("button", { name: "Log" }));
  await screen.findByRole("alert");
  fireEvent.change(subject, { target: { value: "Kickoff with Brandt IT" } });
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(creates(calls)).toBe(2));
  const retry = calls.filter((call) => call.key === "POST /activities")[1];
  expect(retry?.text).toContain('"subject":"Kickoff with Brandt IT"');
});

it("says why the task stays open when the server refuses its completion", async () => {
  const calls = stubApi({
    completionFailures: {
      left: 1,
      status: 403,
      detail: "You may not change this task.",
    },
  });
  renderForm(true);
  await logWith([pdf("outcome.pdf")]);
  expect(
    await screen.findByText(
      "The activity was logged, but the task could not be closed: You may not change this task.",
    ),
  ).toBeTruthy();
  expect(completions(calls)).toHaveLength(1);
  expect(screen.queryByText(/Retry to close it/)).toBeNull();
});

it("sends a backdated entry's changed day after its answer was lost", async () => {
  const calls = stubApi({ lostCreateAnswers: { left: 1 } });
  renderForm();
  const user = userEvent.setup();
  const subject = await screen.findByRole("textbox", { name: "Subject" });
  fireEvent.change(subject, { target: { value: "Kickoff with Brandt IT" } });
  const date = screen.getByRole("textbox", { name: "Date" });
  await user.tripleClick(date);
  await user.keyboard("10/07/2026");
  await user.click(screen.getByRole("button", { name: "Log" }));
  await screen.findByRole("alert");
  await user.tripleClick(date);
  await user.keyboard("03/07/2026");
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(creates(calls)).toBe(2));
  expect(createBodies(calls).map((body) => body.occurred_at)).toEqual([
    "2026-07-10T12:00:00Z",
    "2026-07-03T12:00:00Z",
  ]);
});

it("stamps a fresh time on a retry after the server refused the create", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  const calls = stubApi({ refusedCreates: { left: 1 } });
  renderForm();
  const user = userEvent.setup();
  fireEvent.change(await screen.findByRole("textbox", { name: "Subject" }), {
    target: { value: "Kickoff with Brandt IT" },
  });
  await user.click(screen.getByRole("button", { name: "Log" }));
  await screen.findByRole("alert");
  vi.setSystemTime(Date.now() + 60_000);
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(creates(calls)).toBe(2));
  const [refused, retry] = createBodies(calls);
  expect(retry.occurred_precision).toBe("instant");
  expect(retry.occurred_at).not.toBe(refused.occurred_at);
});
