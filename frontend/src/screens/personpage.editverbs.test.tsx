/** @vitest-environment jsdom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import {
  mount,
  openRecordMenu,
  pressRecordVerb,
  view,
} from "./personpage.testkit";
import { jsonResponse } from "./story-utils";

// The header's edit/merge/archive verbs, restored to the record page every
// contact actually opens to — now rows of its overflow menu rather than
// buttons in the open. Its own file, sharing personpage.testkit.tsx's fixture
// and menu opener with personpage.test.tsx rather than duplicating them.

type Person360 = components["schemas"]["Person360"];

afterEach(() => {
  cleanup();
});

describe("a live contact that is not the viewer's to change refuses its core write verbs", () => {
  const sentence =
    "You cannot change this contact. Ask their owner to share them with you, or your administrator for the right to edit them.";
  const notMine: Person360 = {
    ...view,
    person: { ...view.person, owner_id: "u-other", writable: false },
  };

  it("refuses edit, merge and archive from the same sentence", async () => {
    mount("overview", notMine);

    await screen.findByText(sentence);
    // Refused, not withdrawn: the rows are there to be read and say why they
    // will not press.
    await openRecordMenu();
    for (const testId of ["edit-record", "merge-record", "archive-record"]) {
      const verb = await screen.findByTestId(testId);
      expect(verb.hasAttribute("disabled")).toBe(true);
      expect(
        document.getElementById(verb.getAttribute("aria-describedby") ?? "")
          ?.textContent,
      ).toBe(sentence);
    }
  });
});

describe("the header's core record verbs — edit, merge, archive", () => {
  it("PATCHes /people/{id} from the header's edit form, and the page shows the save — not the stale 360 it had cached", async () => {
    let patchBody: unknown = null;
    let saved = false;
    // version is the field EditAction's own record.version reads, and its
    // absence throws inside the update callback (requireVersion) rather than
    // sending nothing — the shared fixture carries no version because no
    // other spec that imports it writes the record.
    const withVersion: Person360 = {
      ...view,
      person: { ...view.person, version: 3, title: "Old title" },
    };
    mount("overview", withVersion, [], {
      "PATCH /people/p-1": (body) => {
        patchBody = body;
        saved = true;
        return jsonResponse({ ...withVersion.person, title: "New title" });
      },
      // The page reads the person through /360, not through the edit
      // response — a save that invalidates the wrong cache key answers the
      // PATCH correctly and still shows the header title it had before.
      "GET /people/p-1/360": () =>
        jsonResponse(
          saved
            ? {
                ...withVersion,
                person: { ...withVersion.person, title: "New title" },
              }
            : withVersion,
        ),
    });

    const subtitle = () => document.querySelector(".record-sub");
    await waitFor(() => expect(subtitle()?.textContent).toBe("Old title"));
    await pressRecordVerb("edit-record");
    const title = await screen.findByLabelText("Title");
    await userEvent.clear(title);
    await userEvent.type(title, "New title");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({ title: "New title" });
    await waitFor(() => expect(subtitle()?.textContent).toBe("New title"));
  });

  it("archives the record from the header", async () => {
    let archived = false;
    mount("overview", view, [], {
      "DELETE /people/p-1": () => {
        archived = true;
        return jsonResponse({
          ...view.person,
          archived_at: "2026-08-14T00:00:00Z",
        });
      },
    });

    await pressRecordVerb("archive-record");
    await userEvent.click(await screen.findByTestId("archive-confirm"));

    await waitFor(() => expect(archived).toBe(true));
  });

  it("offers merge from the header", async () => {
    mount("overview");

    // Reachability only — MergeAction's own search/confirm flow is exercised
    // fully in contacts.test.tsx, against the identical shared component.
    await openRecordMenu();
    expect(
      (await screen.findByTestId("merge-record")).hasAttribute("disabled"),
    ).toBe(false);
  });
});

// The header's action row, found through the menu that sits in it rather than
// by a class the test spells for itself.
async function headerActions(): Promise<HTMLElement> {
  const trigger = await screen.findByRole("button", {
    name: en["record.moreActions"],
  });
  const row = trigger.closest(".record-actions");
  if (!(row instanceof HTMLElement)) {
    throw new Error("the header's menu does not sit in an action row");
  }
  return row;
}

// The panel the menu's own `aria-controls` names — the same element a screen
// reader is sent to, so a row drawn anywhere else is not in the menu.
async function menuPanel(): Promise<HTMLElement> {
  const trigger = await screen.findByRole("button", {
    name: en["record.moreActions"],
  });
  await userEvent.click(trigger);
  const panel = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  if (!panel) {
    throw new Error("the menu names no panel through aria-controls");
  }
  return panel;
}

describe("every secondary verb is a row of the header's one menu", () => {
  // In the order a reader meets them: the record's own writes, then the
  // quieter doors, then the destructive one last.
  const ROWS = [
    en["record.edit"],
    en["merge.contact"],
    en["record.share"],
    en["record.fullHistory"],
    en["person.action.research"],
    en["record.archive"],
  ];

  it("draws them inside the panel, in order, worded and with no glyph", async () => {
    mount("overview");

    const panel = await menuPanel();
    const rows = await within(panel).findAllByRole("button");
    expect(rows.map((row) => row.textContent)).toEqual(ROWS);
    // A list of named actions with one picture in it is a list with one row
    // the reader has to decode.
    expect(panel.querySelector("svg")).toBeNull();
  });

  it("leaves none of them in the open, and keeps the daily verbs there", async () => {
    mount("overview");

    const actions = await headerActions();
    for (const row of ROWS) {
      expect(within(actions).queryByRole("button", { name: row })).toBeNull();
    }
    // The other direction: a verb a reader came to do must not cost them an
    // opening press.
    for (const daily of [en["log.title"], en["person.action.addTask"]]) {
      expect(within(actions).getByRole("button", { name: daily })).toBeTruthy();
    }
  });
});
