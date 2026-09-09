/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { mount, view } from "./personpage.testkit";
import { jsonResponse } from "./story-utils";

// PersonPageV2 offered no way to correct a bounced email, a wrong phone
// number or a misspelled name — the edit/merge/archive wiring stayed behind
// on the screen nothing routes to any more, and API-only was the only path
// left. Its own file, split from personpage.test.tsx, over the frontend's
// file-length ratchet.

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
  it("PATCHes /people/{id} from the header's edit form", async () => {
    let patchBody: unknown = null;
    // version is the field EditAction's own record.version reads, and its
    // absence throws inside the update callback (requireVersion) rather than
    // sending nothing — the shared fixture carries no version because no
    // other spec that imports it writes the record.
    const withVersion: Person360 = {
      ...view,
      person: { ...view.person, version: 3 },
    };
    mount("overview", withVersion, [], {
      "PATCH /people/p-1": (body) => {
        patchBody = body;
        return jsonResponse({ ...withVersion.person, title: "New title" });
      },
    });

    await waitFor(() => expect(screen.getByTestId("edit-record")).toBeTruthy());
    await userEvent.click(screen.getByTestId("edit-record"));
    const title = await screen.findByLabelText("Title");
    await userEvent.clear(title);
    await userEvent.type(title, "New title");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(patchBody).toBeTruthy());
    expect(patchBody).toMatchObject({ title: "New title" });
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

    await waitFor(() =>
      expect(screen.getByTestId("archive-record")).toBeTruthy(),
    );
    await userEvent.click(screen.getByTestId("archive-record"));
    await userEvent.click(await screen.findByTestId("archive-confirm"));

    await waitFor(() => expect(archived).toBe(true));
  });

  it("offers merge from the header", async () => {
    mount("overview");

    // Reachability only — MergeAction's own search/confirm flow is exercised
    // fully in contacts.test.tsx, against the identical shared component.
    await waitFor(() =>
      expect(screen.getByTestId("merge-record")).toBeTruthy(),
    );
    expect(screen.getByTestId("merge-record").hasAttribute("disabled")).toBe(
      false,
    );
  });
});
