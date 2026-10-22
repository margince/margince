/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { navigate, parseHash, useRoute } from "../../lib/router";
import {
  IMPORT_RUN_SUBPAGE,
  settingsHref,
  settingsRouteTarget,
} from "../../lib/settingsrouting";
import { meFixture } from "../../testing/mefixture";
import { tabContent } from "../settings/settings";
import { ImportCard } from "./import";
import {
  dryRun,
  jsonResponse,
  profile,
  render,
  renderAtImport,
  run,
  stubRoutes,
} from "./import.testkit";

// The settings screen hands the card the segment the shell is showing.
function RoutedImport() {
  return <ImportCard subpage={useRoute().id2} />;
}

const RUN_PAGE = "#/settings/import/run";

const DRAFT_CLOSED =
  "This draft is not offered again. Upload the file again to start over.";

// The run page is a page of its own under the hub. Nothing is on screen until
// /me has answered whether this seat may import at all, so the page is waited
// for rather than looked up.
async function openWizard() {
  navigate({ ...settingsHref("import"), id2: IMPORT_RUN_SUBPAGE });
  await screen.findByRole("link", { name: "Back to data import" });
}

// One CSV dropped on the run page, its type kept, and the Map step open.
async function toTheMap(user: UserEvent, file = "leads.csv") {
  globalThis.location.hash = RUN_PAGE;
  render(<RoutedImport />);
  await user.upload(
    await screen.findByLabelText("Upload your data"),
    new File(["Email\na@x.test\n"], file),
  );
  await user.click(await screen.findByRole("button", { name: /Next: map/ }));
  await screen.findByRole("row", { name: /Notes/ });
}

// The save the test run follows: the last PATCH before the validate.
function staged(sent: ReturnType<typeof stubRoutes>) {
  const validated = sent.findIndex((s) => s.path.endsWith("/validate"));
  if (validated < 0) return undefined;
  return sent
    .slice(0, validated)
    .filter((s) => s.method === "PATCH")
    .at(-1);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

describe("one CSV through the run page", () => {
  it("shows each column's fill rate and values before anyone maps it", async () => {
    const user = userEvent.setup();
    stubRoutes();
    await toTheMap(user);

    // The fill rate is what separates a column worth mapping from one that is
    // a mapping mistake waiting to happen; without it a name is all you have.
    const notes = screen.getByRole("row", { name: /Notes/ });
    expect(within(notes).getByText("25%")).toBeInTheDocument();
    const email = screen.getByRole("row", { name: /Email/ });
    expect(within(email).getByText(/ada@x.test/)).toBeInTheDocument();
    expect(within(notes).getByText("Empty")).toBeInTheDocument();
  });

  it("sends only the columns with a destination, as one object", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    await toTheMap(user);
    await user.click(screen.getByRole("button", { name: "Run test" }));

    await waitFor(() => expect(staged(sent)).toBeDefined());
    // "Notes" was suggested nothing, so it stays out: an unmapped column is
    // not a column mapped to nothing.
    expect(staged(sent)?.body).toEqual({
      version: run.version,
      object: "lead",
      mapping: { Email: "email", "Full Name": "full_name" },
      draft_step: "map",
    });
    expect(
      await screen.findByText(/the "Email" column is empty/),
    ).toBeVisible();
  });

  it("drops a column the human clears, and keeps the ones they kept", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    await toTheMap(user);
    await user.click(
      screen.getByRole("combobox", { name: "Field for Full Name" }),
    );
    await user.click(screen.getByRole("option", { name: "Do not import" }));
    await user.click(screen.getByRole("button", { name: "Run test" }));

    await waitFor(() =>
      expect(staged(sent)?.body).toMatchObject({ mapping: { Email: "email" } }),
    );
  });

  it("shows a column name the file spells oddly, verbatim", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "POST /imports/sources": () =>
        jsonResponse({
          ...profile,
          columns: [
            ...profile.columns,
            { header: "E-Mail (work) ", fill_rate: 1, samples: ["x@y.test"] },
          ],
        }),
    });
    await toTheMap(user);
    expect(
      screen.getByRole("combobox", { name: /^Field for E-Mail \(work\)/ }),
    ).toBeInTheDocument();
  });

  it("writes nothing until the human presses Import", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    await toTheMap(user);
    await user.click(screen.getByRole("button", { name: "Run test" }));
    const commit = await screen.findByRole("button", { name: "Import 3 rows" });

    expect(sent.some((s) => s.path.endsWith("/approve"))).toBe(false);
    await user.click(commit);
    await waitFor(() =>
      expect(sent.some((s) => s.path.endsWith("/approve"))).toBe(true),
    );
  });

  it("counts the rows it will write in words that read as English", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [`GET /imports/${run.id}/report`]: () =>
        jsonResponse({
          ...dryRun,
          disposition: { created: 1, updated: 0, unchanged: 2, skipped: 1 },
        }),
    });
    await toTheMap(user);
    await user.click(screen.getByRole("button", { name: "Run test" }));

    // "1 rows" is how a machine counts. This button is the last thing a human
    // reads before the least reversible write in the product.
    expect(
      await screen.findByRole("button", { name: "Import 1 row" }),
    ).toBeInTheDocument();
  });

  it("uploads the same file again when the reader changes its type", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    globalThis.location.hash = RUN_PAGE;
    render(<RoutedImport />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\n"], "leads.csv"),
    );
    await user.click(
      await screen.findByRole("combobox", {
        name: "Record type for leads.csv",
      }),
    );
    await user.click(screen.getByRole("option", { name: "Companies" }));

    await waitFor(() =>
      expect(
        sent
          .filter((s) => s.path === "POST /imports/sources")
          .map((s) => s.body),
      ).toEqual([
        { object: "lead", file: "leads.csv" },
        { object: "company", file: "leads.csv" },
      ]),
    );
  });

  it("says what went wrong with a file it cannot read", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "POST /imports/sources": () =>
        jsonResponse(
          {
            status: 422,
            code: "validation_error",
            detail: "The uploaded file has no content.",
            details: {
              errors: [
                {
                  field: "file",
                  code: "empty",
                  message: "The uploaded file has no content.",
                },
              ],
            },
          },
          422,
        ),
    });
    globalThis.location.hash = RUN_PAGE;
    render(<RoutedImport />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File([""], "leads.csv"),
    );

    expect(await screen.findByText("File could not be read")).toBeVisible();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("says to split a file with more rows than one test run reads", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "POST /imports/sources": () =>
        jsonResponse(
          {
            status: 422,
            code: "validation_error",
            detail: "The request is not valid.",
            details: {
              errors: [
                {
                  field: "file",
                  code: "too_many_rows",
                  message: "this upload has 15001 rows",
                },
              ],
            },
          },
          422,
        ),
    });
    globalThis.location.hash = RUN_PAGE;
    render(<RoutedImport />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\na@x.test\n"], "leads.csv"),
    );

    expect(
      await screen.findByText("File is too long for one test run"),
    ).toBeVisible();
    expect(
      screen.getByText(
        "This file has more rows than one test run can read. Split it into smaller files and import each one.",
      ),
    ).toBeVisible();
    expect(screen.queryByText("File could not be read")).toBeNull();
  });

  it("refuses an upload answer that names no run", async () => {
    const user = userEvent.setup();
    const { run_id: _unnamed, ...unnamed } = profile;
    stubRoutes({ "POST /imports/sources": () => jsonResponse(unnamed) });
    globalThis.location.hash = RUN_PAGE;
    render(<RoutedImport />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\na@x.test\n"], "leads.csv"),
    );

    expect(await screen.findByText("File could not be read")).toBeVisible();
  });
});

describe("who is offered the import", () => {
  it("is offered to an ops seat, which the server also admits", async () => {
    stubRoutes({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["ops"],
            allow: { import_run: ["create", "update", "read"] },
          }),
        ),
    });
    render(<RoutedImport />);

    // The grant is what decides, not the admin role: an ops seat holds
    // import_run and would be accepted by the store.
    expect(await screen.findByText("New import")).toBeInTheDocument();
  });

  // The flow creates a run, parks it, and approves it: three actions, not one.
  // A role edited to create-without-update would see the card and be refused at
  // the first button it presses.
  it("is not offered to a role that could not finish the flow", async () => {
    stubRoutes({
      "GET /me": () =>
        jsonResponse(
          meFixture({ roles: ["ops"], allow: { import_run: ["create"] } }),
        ),
    });
    const { container } = render(<RoutedImport />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  it("is not offered to a seat that may not run one", async () => {
    stubRoutes({
      "GET /me": () => jsonResponse(meFixture({ roles: ["rep"], allow: {} })),
    });
    const { container } = render(<RoutedImport />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});

describe("a run named by the address", () => {
  it("is read back with the undo it still carries", async () => {
    const sent = stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({ ...run, id: "r1", status: "complete" }),
    });
    renderAtImport("r1");

    expect(
      await screen.findByRole("button", { name: "Undo this import" }),
    ).toBeInTheDocument();
    // Read back, not re-uploaded: nothing about the file is on this machine
    // any more, and the reader chose no file this time.
    expect(sent.some((s) => s.path === "POST /imports/sources")).toBe(false);
  });

  it("names what an undo kept and what it could not reverse", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({ ...run, id: "r1", status: "undone" }),
      "GET /imports/r1/report": () =>
        jsonResponse({
          ...dryRun,
          run_id: "r1",
          status: "undone",
          undo: {
            run_id: "r1",
            status: "undone",
            reversed_count: 1,
            kept: [{ object: "lead", id: "l-1" }],
            errored: [
              {
                object: "company",
                id: "c-1",
                reason: "A contact still references this company.",
              },
            ],
          },
        }),
    });
    renderAtImport("r1");

    expect(await screen.findByText("1 row reversed.")).toBeVisible();
    expect(screen.getByText("Prospects: l-1")).toBeVisible();
    expect(
      screen.getByText(/c-1: A contact still references this company\./),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Undo this import" }),
    ).toBeNull();
  });

  it("says a cancelled run was canceled and starts again from an upload", async () => {
    const user = userEvent.setup();
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({ ...run, id: "r1", status: "cancelled" }),
    });
    renderAtImport("r1");

    expect(await screen.findByText("This import was canceled")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Upload a file" }));
    expect(await screen.findByLabelText("Upload your data")).toBeVisible();
    expect(globalThis.location.hash).not.toContain("run=");
  });

  it("says why a draft's last test stopped when this page no longer holds its file", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({
          ...run,
          id: "r1",
          status: "pending",
          error: "The test run stopped before it finished.",
        }),
    });
    renderAtImport("r1");

    expect(await screen.findByText(DRAFT_CLOSED)).toBeVisible();
    expect(
      screen.getByText("The test run stopped before it finished."),
    ).toBeVisible();
  });

  it("says a draft that never stopped wrote nothing, rather than an empty notice", async () => {
    stubRoutes({
      "GET /imports/r1": () =>
        jsonResponse({ ...run, id: "r1", status: "pending", error: null }),
    });
    renderAtImport("r1");

    expect(await screen.findByText(DRAFT_CLOSED)).toBeVisible();
    expect(
      screen.getByText("This import stopped before anything was imported."),
    ).toBeVisible();
  });

  it("says so when the server will not answer for the run", async () => {
    stubRoutes({
      "GET /imports/r9": () =>
        jsonResponse(
          { status: 404, code: "not_found", detail: "No such import run." },
          404,
        ),
    });
    renderAtImport("r9");

    expect(await screen.findByText("No such import run.")).toBeVisible();
  });

  it("is opened from the hub's list of recent imports", async () => {
    stubRoutes({
      "GET /imports": () =>
        jsonResponse({
          data: [
            { ...run, id: "r2", file_name: "crm.zip", status: "complete" },
          ],
        }),
    });
    render(<RoutedImport />);

    expect(
      await screen.findByRole("link", { name: "crm.zip" }),
    ).toHaveAttribute("href", `${RUN_PAGE}?run=r2`);
  });
});

describe("the import page", () => {
  it("is the settings import entry's own page, reached through its arm", async () => {
    const route = parseHash(RUN_PAGE);
    expect(settingsRouteTarget(route)).toEqual({
      kind: "page",
      page: "import",
      legacy: false,
    });
    stubRoutes();
    render(tabContent("import", route));

    expect(
      await screen.findByRole("link", { name: "Back to data import" }),
    ).toBeInTheDocument();
  });

  it("answers a deep link with the first step and the way back to the hub", async () => {
    globalThis.location.hash = RUN_PAGE;
    stubRoutes();
    render(<RoutedImport />);

    expect(await screen.findByLabelText("Upload your data")).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Back to data import" }),
    ).toHaveAttribute("href", "#/settings/import");
    expect(screen.queryByText("New import")).toBeNull();
  });

  it("returns to the hub on the browser's Back", async () => {
    globalThis.location.hash = "#/settings/import";
    stubRoutes();
    render(<RoutedImport />);
    await openWizard();

    globalThis.history.back();

    expect(await screen.findByText("New import")).toBeInTheDocument();
    expect(globalThis.location.hash).toBe("#/settings/import");
  });

  // The hub's drop zone unmounts as the page opens, so focus would otherwise
  // fall to the body. The file it took is the one the page profiles.
  it("starts the file dropped on the hub, with focus on the page's title", async () => {
    const user = userEvent.setup();
    globalThis.location.hash = "#/settings/import";
    const sent = stubRoutes();
    render(<RoutedImport />);
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["Email\na@x.test\n"], "leads.csv"),
    );

    const title = await screen.findByRole("heading", {
      name: "Check your files",
    });
    expect(within(title).getByText("Check your files")).toHaveFocus();
    await waitFor(() =>
      expect(
        sent.find((s) => s.path === "POST /imports/sources")?.body,
      ).toMatchObject({ file: "leads.csv", object: "lead" }),
    );
  });
});
