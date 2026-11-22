// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  type Contact360,
  mount,
  view,
} from "../../testing/contactpage.testkit";
import { jsonResponse, permissionDenied } from "../../testing/story-utils";
import { refusedRead } from "./contactarchived.fixtures";

// The impersonation warning on the contact page: the contact read carries it,
// and Looks fine clears it.

afterEach(cleanup);

type Warning = NonNullable<Contact360["contact"]["impersonation_warning"]>;

const DETECTED_AT = "2026-08-10T07:30:00Z";

function warned(warning: Warning, writable = true): Contact360 {
  return {
    ...view,
    contact: { ...view.contact, writable, impersonation_warning: warning },
  };
}

const CLEAR_ROUTE = "POST /contacts/p-1/impersonation-warning/clear";

const clearButton = () =>
  screen.findByRole("button", { name: en["contact.impersonation.clear"] });

describe("the impersonation warning on the contact page", () => {
  it.each([
    ["punycode_domain", en["contact.impersonation.reason.punycodeDomain"]],
    [
      "display_name_other_domain",
      en["contact.impersonation.reason.displayNameOtherDomain"],
    ],
    [undefined, en["contact.impersonation.reason.unrecorded"]],
  ] as const)(
    "states the warning with the reason for %s",
    async (reason, sentence) => {
      mount("overview", warned({ reason, detected_at: DETECTED_AT }));

      expect(
        await screen.findByText(en["contact.impersonation.title"]),
      ).toBeTruthy();
      expect(screen.getByText(sentence)).toBeTruthy();
    },
  );

  it("shows no warning on a contact that carries none", async () => {
    mount("overview");

    await screen.findByRole("heading", { level: 1, name: "Dana Buyer" });
    expect(screen.queryByText(en["contact.impersonation.title"])).toBeNull();
  });

  it("clears the warning when Looks fine is pressed", async () => {
    const user = userEvent.setup();
    let page = warned({ reason: "punycode_domain", detected_at: DETECTED_AT });
    const cleared = vi.fn((_body: unknown) => {
      page = view;
      return new Response(null, { status: 204 });
    });
    mount("overview", page, [], {
      "GET /contacts/p-1/360": () => jsonResponse(page),
      [CLEAR_ROUTE]: cleared,
    });

    await user.click(await clearButton());

    await waitFor(() => expect(cleared).toHaveBeenCalledTimes(1));
    expect(cleared.mock.calls[0]?.[0]).toEqual({ detected_at: DETECTED_AT });
    await waitFor(() =>
      expect(screen.queryByText(en["contact.impersonation.title"])).toBeNull(),
    );
  });

  it("keeps the warning and says so when the clear is refused", async () => {
    const user = userEvent.setup();
    mount(
      "overview",
      warned({ reason: "punycode_domain", detected_at: DETECTED_AT }),
      [],
      { [CLEAR_ROUTE]: permissionDenied },
    );

    await user.click(await clearButton());

    expect((await screen.findByRole("alert")).textContent).not.toBe("");
    expect(screen.getByText(en["contact.impersonation.title"])).toBeTruthy();
  });

  // A merge can put another warning on the contact after the page read it.
  it("shows the warning that stands when the one shown was replaced", async () => {
    const user = userEvent.setup();
    let page = warned({ reason: "punycode_domain", detected_at: DETECTED_AT });
    mount("overview", page, [], {
      "GET /contacts/p-1/360": () => jsonResponse(page),
      [CLEAR_ROUTE]: () => {
        page = warned({
          reason: "display_name_other_domain",
          detected_at: "2026-08-12T10:00:00Z",
        });
        return jsonResponse(
          { status: 409, code: "conflict", title: "Conflict" },
          409,
        );
      },
    });

    await user.click(await clearButton());

    expect(
      await screen.findByText(
        en["contact.impersonation.reason.displayNameOtherDomain"],
      ),
    ).toBeTruthy();
    expect(screen.getByRole("alert").textContent).not.toBe("");
  });

  it("refuses the clear to a reader who may not edit the contact", async () => {
    mount(
      "overview",
      warned({ reason: "punycode_domain", detected_at: DETECTED_AT }, false),
    );

    const button = await clearButton();
    await waitFor(() => expect(button).toHaveProperty("disabled", true));
    expect(button.getAttribute("aria-describedby")).toBeTruthy();
  });

  // The warning is part of the contact read. A read the server refuses shows
  // the page's own sentence and nothing about a warning.
  it("shows no warning when the contact read is refused", async () => {
    mount(
      "overview",
      warned({ reason: "punycode_domain", detected_at: DETECTED_AT }),
      [],
      {
        "GET /contacts/p-1": refusedRead,
        "GET /contacts/p-1/360": refusedRead,
      },
    );

    expect(await screen.findByText(en["record.gone.contact"])).toBeTruthy();
    expect(screen.queryByText(en["contact.impersonation.title"])).toBeNull();
  });
});
