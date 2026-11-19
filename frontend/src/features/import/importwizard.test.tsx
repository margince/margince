/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS } from "../../design-system/debouncedsearch";
import { ImportCard } from "./import";
import {
  jsonResponse,
  profile,
  render,
  renderAtImport,
  run,
  stubRoutes,
} from "./import.testkit";

const RUN_PAGE = "#/settings/import/run";
const RUN = `/imports/${run.id}`;

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

// One CSV dropped on the run page, and the Map step open.
async function toTheMap(user: UserEvent) {
  globalThis.location.hash = RUN_PAGE;
  render(<ImportCard subpage="run" />);
  await user.upload(
    await screen.findByLabelText("Upload your data"),
    new File(["Email\na@x.test\n"], "leads.csv"),
  );
  await user.click(await screen.findByRole("button", { name: /Next: map/ }));
  await screen.findByRole("row", { name: /Notes/ });
}

async function clearFullName(user: UserEvent) {
  await user.click(
    screen.getByRole("combobox", { name: "Field for Full Name" }),
  );
  await user.click(screen.getByRole("option", { name: "Do not import" }));
}

// Holds every request whose url ends in `tail`, or every PATCH, until released.
// A test then sees what is sent while one is in flight.
function holdRequests(tail?: string) {
  const inner = globalThis.fetch;
  const started: string[] = [];
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request?.url ?? input);
      const held = tail
        ? url.endsWith(tail)
        : (request?.method ?? init?.method) === "PATCH";
      if (held) {
        started.push(url);
        await gate;
      }
      return inner(input, init);
    }),
  );
  return { started, release };
}

describe("the wizard is the run", () => {
  it("reload at Map shows the saved step and the file's type", async () => {
    stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({
          ...run,
          status: "pending",
          draft_step: "map",
          profile,
        }),
    });
    renderAtImport(run.id);
    expect(await screen.findByRole("row", { name: /Notes/ })).toBeVisible();
    expect(screen.getByRole("button", { name: "Run test" })).toBeEnabled();
  });

  it("reload at Files shows the files step, not the map", async () => {
    stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({
          ...run,
          status: "pending",
          draft_step: "files",
          profile,
        }),
    });
    renderAtImport(run.id);
    expect(await screen.findByLabelText("Source name")).toBeVisible();
  });

  it("two quick edits make no conflict", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      let version = run.version;
      const sent = stubRoutes({
        [`PATCH ${RUN}`]: () =>
          jsonResponse({ ...run, status: "pending", version: ++version }),
      });
      await toTheMap(user);
      const held = holdRequests();
      await clearFullName(user);
      await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
      await waitFor(() => expect(held.started).toHaveLength(1));
      await user.click(
        screen.getByRole("combobox", { name: "Field for Email" }),
      );
      await user.click(screen.getByRole("option", { name: "Do not import" }));
      await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2);
      expect(held.started).toHaveLength(1);
      held.release();
      await waitFor(() =>
        expect(sent.filter((s) => s.method === "PATCH")).toHaveLength(2),
      );
      const versions = sent
        .filter((s) => s.method === "PATCH")
        .map((s) => (s.body as { version: number }).version);
      expect(versions[1]).toBeGreaterThan(versions[0]);
      expect(screen.queryByText(/changed in another tab/)).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("Test run right after an edit saves that edit and validates its version", async () => {
    const user = userEvent.setup();
    let version = run.version;
    const sent = stubRoutes({
      [`PATCH ${RUN}`]: () =>
        jsonResponse({ ...run, status: "pending", version: ++version }),
    });
    await toTheMap(user);
    await clearFullName(user);
    await user.click(screen.getByRole("button", { name: "Run test" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(`POST ${RUN}/validate`),
    );
    const writes = sent.filter((s) => s.method !== "GET");
    const patch = writes.filter((s) => s.method === "PATCH").pop();
    expect(patch?.body).toMatchObject({
      object: "lead",
      mapping: { Email: "email" },
      draft_step: "map",
    });
    const validate = writes
      .filter((s) => s.path === `POST ${RUN}/validate`)
      .pop();
    expect(validate?.body).toEqual({ version });
  });

  it("a version conflict from another tab refetches and says so", async () => {
    const user = userEvent.setup();
    let reads = 0;
    stubRoutes({
      [`PATCH ${RUN}`]: () =>
        jsonResponse(
          { status: 409, code: "conflict", detail: "Changed." },
          409,
        ),
    });
    await toTheMap(user);
    const before = vi
      .mocked(fetch)
      .mock.calls.filter(([i]) =>
        String((i as Request).url).includes(RUN),
      ).length;
    await clearFullName(user);
    expect(await screen.findByText(/changed in another tab/)).toBeVisible();
    expect(
      screen
        .getAllByRole("alert")
        .some((one) => /another tab/.test(one.textContent ?? "")),
    ).toBe(true);
    await user.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText(/changed in another tab/)).toBeNull();
    await waitFor(() => {
      reads = vi
        .mocked(fetch)
        .mock.calls.filter(([i]) =>
          String((i as Request).url).includes(RUN),
        ).length;
      expect(reads).toBeGreaterThan(before + 1);
    });
  });

  it("a refused save drops the edits it carried and sends nothing more", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      let refused = false;
      const sent = stubRoutes({
        [`PATCH ${RUN}`]: () => {
          refused = true;
          return jsonResponse(
            { status: 409, code: "conflict", detail: "Changed." },
            409,
          );
        },
      });
      // The stub serves the draft it was uploaded as; the other tab's save is
      // what a read answers once this tab's save is refused.
      const stub = globalThis.fetch;
      const saved = {
        ...run,
        status: "pending",
        draft_step: "map",
        version: 4,
        profile,
        parts: [
          {
            file: "leads.csv",
            object: "lead",
            row_count: 4,
            mapping: { Email: "email", "Full Name": "title" },
          },
        ],
      };
      vi.stubGlobal(
        "fetch",
        vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
          const request = input instanceof Request ? input : null;
          const read =
            (request?.method ?? "GET") === "GET" &&
            request?.url.endsWith(RUN) === true;
          return refused && read ? jsonResponse(saved) : stub(input, init);
        }),
      );
      await toTheMap(user);
      await clearFullName(user);
      await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
      await screen.findByText(/changed in another tab/);
      await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 4);
      expect(sent.filter((s) => s.method === "PATCH")).toHaveLength(1);
      await waitFor(() =>
        expect(
          screen.getByRole("combobox", { name: "Field for Full Name" }),
        ).toHaveTextContent(/title/i),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it("a superseded run offers the newer one", async () => {
    stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({
          ...run,
          status: "cancelled",
          superseded_by: "019ff-newer",
        }),
    });
    renderAtImport(run.id);
    expect(
      await screen.findByText("A newer test run replaced this import"),
    ).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Open the newer test run" }),
    ).toHaveAttribute("href", expect.stringContaining("run=019ff-newer"));
  });

  it("Cancel on a draft cancels it and returns to the hub", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [`POST ${RUN}/cancel`]: () =>
        jsonResponse({ ...run, status: "cancelled" }),
    });
    globalThis.location.hash = RUN_PAGE;
    render(<ImportCard subpage="run" />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\na@x.test\n"], "leads.csv"),
    );
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(`POST ${RUN}/cancel`),
    );
    await waitFor(() => expect(globalThis.location.hash).not.toContain("run="));
  });

  it("a refused Cancel says so and stays on the draft", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [`POST ${RUN}/cancel`]: () =>
        jsonResponse({ status: 403, code: "permission_denied" }, 403),
    });
    globalThis.location.hash = RUN_PAGE;
    render(<ImportCard subpage="run" />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\na@x.test\n"], "leads.csv"),
    );
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(
      await screen.findByText("The import was not canceled"),
    ).toBeVisible();
    expect(globalThis.location.hash).toContain("run=");
  });

  it("Cancel of a draft another tab is testing stops that test and stays", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [`POST ${RUN}/cancel`]: () => jsonResponse({ ...run, status: "pending" }),
    });
    globalThis.location.hash = RUN_PAGE;
    render(<ImportCard subpage="run" />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\na@x.test\n"], "leads.csv"),
    );
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(`POST ${RUN}/cancel`),
    );
    expect(globalThis.location.hash).toContain("run=");
  });
});

// A single CSV on the Files step, uploaded as `name`, with the server's first
// profile `first` and every later one `then`.
async function dropCsv(
  user: UserEvent,
  name: string,
  first: object,
  then: object = first,
) {
  let uploads = 0;
  const sent = stubRoutes({
    "POST /imports/sources": () => jsonResponse(uploads++ ? then : first),
  });
  globalThis.location.hash = RUN_PAGE;
  render(<ImportCard subpage="run" />);
  await user.upload(
    await screen.findByLabelText("Upload your data"),
    new File(["Email\na@x.test\n"], name),
  );
  await screen.findByLabelText("Source name");
  return sent;
}

const uploadsOf = (sent: ReturnType<typeof stubRoutes>) =>
  sent.filter((s) => s.path === "POST /imports/sources").map((s) => s.body);

const NEXT = { name: /Next: map/ };

describe("another type for a single CSV", () => {
  const retyped = { ...profile, run_id: "019ff-retyped", object: "contact" };

  async function retype(user: UserEvent) {
    await user.click(
      screen.getByRole("combobox", { name: "Record type for leads.csv" }),
    );
    await user.click(screen.getByRole("option", { name: "Contacts" }));
  }

  it("cancels the draft the new upload replaces and opens the new one", async () => {
    const user = userEvent.setup();
    const sent = await dropCsv(user, "leads.csv", profile, retyped);
    await retype(user);
    await waitFor(() =>
      expect(sent.map((s) => s.path)).toContain(`POST ${RUN}/cancel`),
    );
    expect(uploadsOf(sent)).toEqual([
      { object: "lead", file: "leads.csv" },
      { object: "contact", file: "leads.csv" },
    ]);
    expect(globalThis.location.hash).toContain("run=019ff-retyped");
  });

  it("keeps the new draft and says so when the old one is not canceled", async () => {
    const user = userEvent.setup();
    let uploads = 0;
    stubRoutes({
      "POST /imports/sources": () =>
        jsonResponse(uploads++ ? retyped : profile),
      [`POST ${RUN}/cancel`]: () =>
        jsonResponse({ status: 403, code: "permission_denied" }, 403),
    });
    globalThis.location.hash = RUN_PAGE;
    render(<ImportCard subpage="run" />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\n"], "leads.csv"),
    );
    await retype(user);
    expect(
      await screen.findByText(
        "The earlier draft of this file was not canceled",
      ),
    ).toBeVisible();
    expect(globalThis.location.hash).toContain("run=019ff-retyped");
    expect(screen.getByRole("button", NEXT)).toBeEnabled();
  });

  it("picking the type the file was read as uploads nothing again", async () => {
    const user = userEvent.setup();
    const sent = await dropCsv(user, "leads.csv", profile);
    await user.click(
      screen.getByRole("combobox", { name: "Record type for leads.csv" }),
    );
    await user.click(screen.getByRole("option", { name: "Leads" }));
    expect(uploadsOf(sent)).toHaveLength(1);
    expect(sent.some((s) => s.path.endsWith("/cancel"))).toBe(false);
  });
});

describe("a single CSV whose name names no type", () => {
  const unnamed = (object: string, chosenBy?: string) => ({
    ...profile,
    file_name: "export.csv",
    object,
    ...(chosenBy ? { object_chosen_by: chosenBy } : null),
  });
  const typeOf = () =>
    screen.getByRole("combobox", { name: "Record type for export.csv" });

  // The draft as a reload or Continue reads it back from the server.
  function reloadDraft(object: string, chosen: object) {
    stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({
          ...run,
          status: "pending",
          draft_step: "files",
          object,
          profile: chosen,
        }),
    });
    renderAtImport(run.id);
  }

  it("is sent once with no type, and shows the type its header chose", async () => {
    const user = userEvent.setup();
    const sent = await dropCsv(
      user,
      "export.csv",
      unnamed("contact", "columns"),
    );
    expect(uploadsOf(sent)).toEqual([{ file: "export.csv" }]);
    expect(typeOf()).toHaveTextContent("Contacts");
    expect(
      screen.getByText("Record type chosen from the columns"),
    ).toBeVisible();
    expect(screen.getByRole("button", NEXT)).toBeEnabled();
    expect(sent.some((s) => s.path.endsWith("/cancel"))).toBe(false);
  });

  it("opens with no type when its header names none, and refuses Next until one is picked", async () => {
    const user = userEvent.setup();
    const sent = await dropCsv(user, "export.csv", unnamed("lead", "none"));
    expect(screen.getByText("No record type found")).toBeVisible();
    expect(typeOf()).toHaveTextContent("Skip this file");
    expect(screen.getByRole("button", NEXT)).toBeDisabled();
    expect(
      screen.getByText("Choose a record type for at least one file."),
    ).toBeVisible();
    await user.click(typeOf());
    await user.click(screen.getByRole("option", { name: "Leads" }));
    expect(screen.queryByText("No record type found")).toBeNull();
    expect(screen.getByRole("button", NEXT)).toBeEnabled();
    await waitFor(() =>
      expect(sent.find((s) => s.method === "PATCH")?.body).toMatchObject({
        object: "lead",
      }),
    );
    expect(uploadsOf(sent)).toEqual([{ file: "export.csv" }]);
    expect(sent.some((s) => s.path.endsWith("/cancel"))).toBe(false);
  });

  it("still shows no type after a reload of the draft", async () => {
    reloadDraft("lead", unnamed("lead", "none"));
    expect(await screen.findByText("No record type found")).toBeVisible();
    expect(screen.getByRole("button", NEXT)).toBeDisabled();
  });

  it("shows Leads after a reload once the reader saved Leads as the type", async () => {
    reloadDraft("lead", unnamed("lead"));
    await waitFor(() => expect(typeOf()).toHaveTextContent("Leads"));
    expect(screen.queryByText("No record type found")).toBeNull();
    expect(screen.getByRole("button", NEXT)).toBeEnabled();
  });

  it("still names the header's choice after a reload of the draft", async () => {
    reloadDraft("contact", unnamed("contact", "columns"));
    expect(
      await screen.findByText("Record type chosen from the columns"),
    ).toBeVisible();
    expect(typeOf()).toHaveTextContent("Contacts");
  });

  it("shows the type the reader saved over the server's none", async () => {
    reloadDraft("contact", unnamed("lead", "none"));
    await waitFor(() => expect(typeOf()).toHaveTextContent("Contacts"));
    expect(screen.queryByText("No record type found")).toBeNull();
    expect(screen.getByRole("button", NEXT)).toBeEnabled();
  });
});

describe("a run address that names no run", () => {
  it.each([
    [404, "019ff-gone"],
    [422, "not-a-uuid"],
  ])(
    "answering %i says the import is not there, with no retry",
    async (status, id) => {
      stubRoutes({
        [`GET /imports/${id}`]: () =>
          jsonResponse(
            { status, detail: `import run ${id}: not found` },
            status,
          ),
      });
      renderAtImport(id);
      expect(
        await screen.findByText("This import does not exist or was removed."),
      ).toBeVisible();
      expect(screen.queryByText(/A test run is staged/)).toBeNull();
      expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
      expect(screen.queryByText(/Reload the page/)).toBeNull();
      expect(
        screen.getByRole("link", { name: "Back to data import" }),
      ).toBeVisible();
    },
  );
});

describe("draft saves around a test run and a cancel", () => {
  const writesOf = (sent: ReturnType<typeof stubRoutes>) =>
    sent.filter((s) => s.method !== "GET").map((s) => s.path);

  // Runs the fake clock and every task it starts, the renders included.
  const tick = (ms: number) =>
    act(async () => {
      await vi.advanceTimersByTimeAsync(ms);
    });

  // The page is reached on the real clock. From the first edit on, only the
  // test moves time.
  async function onFilesStep(routes: Parameters<typeof stubRoutes>[0] = {}) {
    const user = userEvent.setup();
    const sent = stubRoutes(routes);
    globalThis.location.hash = RUN_PAGE;
    render(<ImportCard subpage="run" />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\n"], "leads.csv"),
    );
    await screen.findByLabelText("Source name");
    vi.useFakeTimers();
    return sent;
  }

  const typeName = (value: string) =>
    fireEvent.change(screen.getByLabelText("Source name"), {
      target: { value },
    });

  // The test run's answer is held. The run then still reads as a draft while
  // the edit Next made has not settled.
  it("Run test right after Next sends no draft save but its own", async () => {
    let version = run.version;
    const sent = await onFilesStep({
      [`PATCH ${RUN}`]: () =>
        jsonResponse({ ...run, status: "pending", version: ++version }),
    });
    const validate = holdRequests(`${RUN}/validate`);
    let before = 0;
    try {
      typeName("crm");
      await tick(SEARCH_DEBOUNCE_MS);
      expect(writesOf(sent)).toContain(`PATCH ${RUN}`);
      before = sent.length;
      fireEvent.click(screen.getByRole("button", NEXT));
      fireEvent.click(screen.getByRole("button", { name: "Run test" }));
      await tick(0);
      expect(validate.started).toHaveLength(1);
      await tick(SEARCH_DEBOUNCE_MS * 2);
      validate.release();
    } finally {
      vi.useRealTimers();
    }
    expect(
      await screen.findByText(/the "Email" column is empty/),
    ).toBeVisible();
    const after = sent.slice(before).filter((s) => s.method !== "GET");
    expect(after.map((s) => s.path)).toEqual([
      `PATCH ${RUN}`,
      `POST ${RUN}/validate`,
    ]);
    expect(after[0]?.body).toMatchObject({
      source_name: "crm",
      draft_step: "map",
    });
    expect(screen.queryByText(/changed in another tab/)).toBeNull();
  });

  // The cancel's answer is held while the clock runs past the edits' settling.
  it("no draft save is sent while a cancel of the draft is on its way", async () => {
    const sent = await onFilesStep({
      [`POST ${RUN}/cancel`]: () =>
        jsonResponse({ ...run, status: "cancelled" }),
    });
    try {
      const cancel = holdRequests(`${RUN}/cancel`);
      typeName("crm");
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      typeName("crm2");
      await tick(0);
      expect(cancel.started).toHaveLength(1);
      await tick(SEARCH_DEBOUNCE_MS * 2);
      expect(writesOf(sent).filter((path) => path.startsWith("PATCH"))).toEqual(
        [],
      );
      cancel.release();
    } finally {
      vi.useRealTimers();
    }
    await waitFor(() => expect(globalThis.location.hash).not.toContain("run="));
    expect(writesOf(sent).filter((path) => path.startsWith("PATCH"))).toEqual(
      [],
    );
  });

  it("a cancelled run sends no draft save", async () => {
    const sent = stubRoutes({
      [`GET ${RUN}`]: () =>
        jsonResponse({ ...run, status: "cancelled", draft_step: "files" }),
    });
    renderAtImport(run.id);
    expect(await screen.findByText("This import was canceled")).toBeVisible();
    expect(writesOf(sent)).toEqual([]);
  });
});

describe("a draft another tab has tested or cancelled", () => {
  it("a save it refuses reads the run again and shows what it is", async () => {
    const user = userEvent.setup();
    let refused = false;
    stubRoutes({
      [`PATCH ${RUN}`]: () => {
        refused = true;
        return jsonResponse(
          {
            status: 409,
            code: "import_run_state",
            detail: "this import is cancelled, so it cannot become pending",
          },
          409,
        );
      },
    });
    const stub = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        const read =
          (request?.method ?? "GET") === "GET" &&
          request?.url.endsWith(RUN) === true;
        return refused && read
          ? jsonResponse({ ...run, status: "cancelled" })
          : stub(input, init);
      }),
    );
    await toTheMap(user);
    await clearFullName(user);
    expect(await screen.findByText("This import was canceled")).toBeVisible();
    expect(screen.queryByText(/changed in another tab/)).toBeNull();
    expect(screen.queryByText(/cannot become pending/)).toBeNull();
  });
});

describe("an edit refused while another tab tested the run", () => {
  // The first save is refused as the run was being tested. Read again, the run
  // is a draft once more, at the version that test left it on.
  it("goes again with the run's new version once the run is a draft", async () => {
    const user = userEvent.setup();
    let refused = false;
    const sent = stubRoutes({
      [`PATCH ${RUN}`]: () => {
        if (refused) {
          return jsonResponse({ ...run, status: "pending", version: 8 });
        }
        refused = true;
        return jsonResponse(
          { status: 409, code: "import_run_state", detail: "being tested" },
          409,
        );
      },
    });
    const stub = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        const read =
          (request?.method ?? "GET") === "GET" &&
          request?.url.endsWith(RUN) === true;
        return refused && read
          ? jsonResponse({
              ...run,
              status: "pending",
              draft_step: "map",
              version: 7,
              profile,
            })
          : stub(input, init);
      }),
    );
    await toTheMap(user);
    await clearFullName(user);
    await waitFor(() =>
      expect(sent.filter((s) => s.method === "PATCH")).toHaveLength(2),
    );
    const [first, again] = sent.filter((s) => s.method === "PATCH");
    expect(first?.body).toMatchObject({ mapping: { Email: "email" } });
    expect(again?.body).toMatchObject({
      mapping: { Email: "email" },
      version: 7,
    });
    expect(screen.queryByText(/changed in another tab/)).toBeNull();
  });
});

describe("the draft after a test run is asked for", () => {
  // Reads of the run are held once the test run has answered. The page then
  // has the test's answer, but not the run read again.
  it("offers no edit while the run is read again", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    const stub = globalThis.fetch;
    let validated = false;
    let release: () => void = () => undefined;
    const reread = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        const url = request?.url ?? String(input);
        if (validated && request?.method === "GET" && url.endsWith(RUN)) {
          await reread;
        }
        const answer = await stub(input, init);
        if (url.endsWith(`${RUN}/validate`)) validated = true;
        return answer;
      }),
    );
    await toTheMap(user);
    await user.click(screen.getByRole("button", { name: "Run test" }));
    await waitFor(() =>
      expect(
        screen.queryByRole("combobox", { name: "Field for Full Name" }),
      ).toBeNull(),
    );
    const validate = sent.findIndex((s) => s.path.endsWith("/validate"));
    release();
    expect(
      await screen.findByText(/the "Email" column is empty/),
    ).toBeVisible();
    expect(sent.slice(validate).filter((s) => s.method === "PATCH")).toEqual(
      [],
    );
  });
});

describe("the hint under the step ladder", () => {
  it.each([
    ["cancelled", "This import was canceled"],
    ["failed", "Importing"],
    ["running", "Importing"],
  ])("says nothing is staged for a run that is %s", async (status, shown) => {
    stubRoutes({
      [`GET ${RUN}`]: () => jsonResponse({ ...run, status }),
    });
    renderAtImport(run.id);
    expect(await screen.findByText(shown)).toBeVisible();
    expect(screen.queryByText(/A test run is staged/)).toBeNull();
    expect(screen.queryByText(/This import is finished/)).toBeNull();
  });

  it("says the files and map are fixed for a staged test run", async () => {
    stubRoutes();
    renderAtImport(run.id);
    expect(
      await screen.findByText(/A test run is staged, so the files and the map/),
    ).toBeVisible();
  });
});
