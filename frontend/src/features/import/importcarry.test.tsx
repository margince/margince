/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { steppedClock } from "../../testing/steppedclock";
import {
  jsonResponse,
  profile,
  renderAtImport,
  run,
  type Sent,
  stubRoutes,
  zipProfile,
} from "./import.testkit";
import { carriedEdits, carriedObject, choicesToCarry } from "./importcarry";
import { filesOf } from "./importflow";
import { asProfile } from "./importprofile";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  globalThis.location.hash = "";
});

const TAG = "0190aaaa-0000-7000-8000-000000000001";
const MAPPING = { Email: "email", Notes: "ref" };

// A CSV whose name names no type, tested as contacts under a source name and a
// tag, with one column mapped by hand.
const tested = {
  ...run,
  object: "contact",
  file_name: "export.csv",
  source_name: "crm-2026",
  context_tag_id: TAG,
  parts: [
    { file: "export.csv", object: "contact", row_count: 4, mapping: MAPPING },
  ],
  profile: { ...profile, file_name: "export.csv", object: "contact" },
};

// What the server answers for the same file again: it types it by its header.
const again = {
  ...profile,
  run_id: "019ff-again",
  file_name: "export.csv",
  object: "lead",
  object_chosen_by: "columns",
};
const asContacts = { ...again, run_id: "019ff-contacts", object: "contact" };

const uploadsOf = (sent: readonly Sent[]) =>
  sent.filter((s) => s.path === "POST /imports/sources").map((s) => s.body);

async function fixAndReupload(user: UserEvent, uploads: readonly object[]) {
  let answered = 0;
  const sent = stubRoutes({
    [`GET /imports/${run.id}`]: () => jsonResponse(tested),
    "POST /imports/sources": () =>
      jsonResponse(uploads[Math.min(answered++, uploads.length - 1)]),
  });
  renderAtImport(run.id);
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["import.test.reupload"],
    }),
  );
  await user.upload(
    await screen.findByLabelText(englishCopy["import.hub.uploadLabel"]),
    new File(["Email\nada@x.test\n"], "export.csv"),
  );
  await screen.findByLabelText("Source name");
  return sent;
}

describe("Fix and re-upload with the same headers", () => {
  it("opens the new file with the type, source name, tag and mapping of the run it replaces", async () => {
    const user = steppedClock();
    const sent = await fixAndReupload(user, [again, asContacts]);
    expect(uploadsOf(sent)).toEqual([
      { file: "export.csv" },
      { object: "contact", file: "export.csv" },
    ]);
    expect(
      screen.getByRole("combobox", { name: "Record type for export.csv" }),
    ).toHaveTextContent("Contacts");
    expect(screen.getByLabelText("Source name")).toHaveValue("crm-2026");
    expect(sent.map((s) => s.path)).toContain(
      "POST /imports/019ff-again/cancel",
    );
    await waitFor(() =>
      expect(
        sent.find((s) => s.path === "PATCH /imports/019ff-contacts")?.body,
      ).toMatchObject({
        object: "contact",
        mapping: MAPPING,
        source_name: "crm-2026",
        context_tag_id: TAG,
      }),
    );
    await user.click(screen.getByRole("button", { name: /Next: map/ }));
    expect(
      await screen.findByRole("combobox", { name: "Field for Notes" }),
    ).toHaveTextContent("ref");
  });

  it("uploads once when the server reads the file as the same type", async () => {
    const user = userEvent.setup();
    const sent = await fixAndReupload(user, [asContacts]);
    expect(uploadsOf(sent)).toEqual([{ file: "export.csv" }]);
    expect(sent.some((s) => s.path.endsWith("/cancel"))).toBe(false);
    expect(screen.getByLabelText("Source name")).toHaveValue("crm-2026");
  });

  it("starts from the server's proposal when the headers differ", async () => {
    const user = steppedClock();
    const other = {
      ...again,
      columns: [{ header: "Company", fill_rate: 1, samples: ["Acme"] }],
      suggested_mapping: { Company: "company_name" },
    };
    const sent = await fixAndReupload(user, [other]);
    expect(uploadsOf(sent)).toEqual([{ file: "export.csv" }]);
    expect(
      screen.getByRole("combobox", { name: "Record type for export.csv" }),
    ).toHaveTextContent("Leads");
    expect(screen.getByLabelText("Source name")).toHaveValue("");
    await user.click(screen.getByRole("button", { name: /Next: map/ }));
    await screen.findByRole("combobox", { name: "Field for Company" });
    await waitFor(() =>
      expect(
        sent.filter((s) => s.method === "PATCH").map((s) => s.body),
      ).toEqual([expect.objectContaining({ draft_step: "map" })]),
    );
    expect(sent.find((s) => s.method === "PATCH")?.body).not.toHaveProperty(
      "source_name",
    );
  });
});

describe("the choices a ZIP carries", () => {
  const zip = asProfile(zipProfile);
  const files = filesOf(zip);
  const carried = choicesToCarry(files, true, {
    objects: { "companies.csv": "company", "contacts.csv": "lead" },
    mapping: {
      "companies.csv": { ref: "ref" },
      "contacts.csv": { email: "email" },
    },
    sourceName: "pipedrive",
    contextTagID: "",
  });

  it("go to the parts of the same name and headers, each changed in place", () => {
    if (!carried) throw new Error("a profile with files carries its choices");
    expect(carriedObject(carried, zip)).toBeUndefined();
    expect(carriedEdits(carried, zip)).toEqual({
      objects: { "companies.csv": "company", "contacts.csv": "lead" },
      mapping: {
        "companies.csv": { ref: "ref" },
        "contacts.csv": { email: "email" },
      },
      sourceName: "pipedrive",
    });
  });

  it("are dropped when one part has other headers, or the upload is one CSV", () => {
    if (!carried) throw new Error("a profile with files carries its choices");
    const [companies, contacts] = zip.parts ?? [];
    const changed = {
      ...zip,
      parts: [companies, { ...contacts, columns: contacts.columns.slice(1) }],
    };
    expect(carriedEdits(carried, changed)).toBeNull();
    expect(carriedEdits(carried, asProfile(profile))).toBeNull();
  });

  it("are none for a run whose file is no longer read", () => {
    expect(
      choicesToCarry([], false, {
        objects: {},
        mapping: {},
        sourceName: "crm",
        contextTagID: "",
      }),
    ).toBeNull();
  });
});
