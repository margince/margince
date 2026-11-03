/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { formatFixture } from "../../testing/import.fixtures";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { ContactActions } from "../contacts/contactactions";
import { ImportCard } from "./import";
import { jsonResponse, render, stubRoutes } from "./import.testkit";

const CONTACT = "c-ada";
const RUN_PAGE = "#/settings/import/run";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

const activityProfile = {
  run_id: "019ff-run",
  source_ref: "ws/import/acts",
  object: "activity",
  file_name: "calls.csv",
  rows_profiled: 2,
  row_count: 2,
  columns: [
    { header: "subject", fill_rate: 1, samples: ["Call"] },
    { header: "occurred_at", fill_rate: 1, samples: ["2025-03-02"] },
  ],
  suggested_mapping: { subject: "subject", occurred_at: "occurred_at" },
  targets: ["subject", "occurred_at", "links"],
};

const withActivities = {
  ...formatFixture,
  order: [...formatFixture.order, "activity"],
  files: [
    ...formatFixture.files,
    {
      file: "activities.csv",
      object: "activity",
      columns: ["subject", "occurred_at", "links"].map((name) => ({
        name,
        description: `The activity's ${name}.`,
        required_on_new_row: name !== "subject",
        custom: false,
      })),
    },
  ],
};

describe("importing activities from a contact's page", () => {
  it("opens the import on a contact with the type preset and no links column needed", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["admin"],
            allow: { import_run: ["create", "update", "read"] },
          }),
        ),
      "GET /imports/format": () => jsonResponse(withActivities),
      [`GET /contacts/${CONTACT}`]: () =>
        jsonResponse({ id: CONTACT, full_name: "Ada Lovelace" }),
      "POST /imports/sources": () => jsonResponse(activityProfile),
    });
    globalThis.location.hash = `${RUN_PAGE}?object=activity&contact=${CONTACT}`;
    render(<ImportCard subpage="run" />);

    expect(
      await screen.findByText("Importing into Ada Lovelace"),
    ).toBeVisible();
    await user.upload(
      await screen.findByLabelText("Upload your data"),
      new File(["subject,occurred_at\nCall,2025-03-02\n"], "calls.csv"),
    );
    await user.click(await screen.findByRole("button", { name: /Next: map/ }));
    await user.click(await screen.findByRole("button", { name: "Run test" }));

    const upload = sent.find((one) => one.path === "POST /imports/sources");
    expect(JSON.stringify(upload?.body)).toContain("activity");
    const saves = await waitFor(() => {
      const patches = sent
        .filter((one) => one.path.startsWith("PATCH /imports/"))
        .map((one) => one.body);
      expect(patches).toContainEqual(
        expect.objectContaining({
          preset_links: [{ type: "contact", id: CONTACT }],
        }),
      );
      return patches;
    });
    expect(
      JSON.stringify(saves),
      "no links column is mapped to a target",
    ).not.toContain('"links"');
  });

  it("shows Import activities only to a reader who may import", async () => {
    const user = userEvent.setup();
    const mount = (allow: GrantSpec) => {
      stubRoutes({
        "GET /me": () => jsonResponse(meFixture({ roles: ["rep"], allow })),
      });
      return render(
        <ContactActions
          view={{ contact: { id: CONTACT } } as never}
          contactId={CONTACT}
          onWrite={() => undefined}
          onResearch={() => undefined}
          onLogActivity={() => undefined}
          onAddTask={() => undefined}
        />,
      );
    };
    const openMenu = async () => {
      await user.click(
        await screen.findByRole("button", { name: en["record.moreActions"] }),
      );
      await screen.findByText(en["record.fullHistory"]);
    };

    mount({ import_run: ["read"], contact: ["read"] });
    await openMenu();
    expect(
      screen.queryByText(en["contact.action.importActivities"]),
    ).toBeNull();
    cleanup();

    mount({ import_run: ["create", "update", "read"], contact: ["read"] });
    await openMenu();
    expect(
      await screen.findByText(en["contact.action.importActivities"]),
    ).toBeVisible();
  });
});
