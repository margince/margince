/** @vitest-environment happy-dom */
import { focusManager } from "@tanstack/react-query";
import { act, cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { mount, view } from "./contactpage.testkit";
import { jsonResponse } from "./story-utils";

// A draft an agent left for the reader waits on the contact's page, and opening
// it opens the page's own composer with the saved words in it.

const AGENT_DRAFT: components["schemas"]["MailDraft"] = {
  id: "md-7",
  anchor_type: "contact",
  anchor_id: "p-1",
  to: ["dana@brandt.example"],
  cc: [],
  bcc: [],
  subject: "After the fleet demo",
  body: "Thank you for the time on Tuesday.",
  html_body: null,
  version: 1,
  agent_drafted: true,
  created_at: "2026-08-12T09:00:00Z",
  updated_at: "2026-08-12T09:00:00Z",
};

afterEach(() => {
  cleanup();
  focusManager.setFocused(undefined);
});

describe("a draft waiting on the contact", () => {
  it("is shown as an agent's and opens the composer holding it", async () => {
    const user = userEvent.setup();
    mount("overview", view, [], {
      "GET /mail-drafts": () => jsonResponse(AGENT_DRAFT),
    });

    const notice = (
      await screen.findByText(en["compose.waitingDraftTitle"])
    ).closest(".callout");
    if (!(notice instanceof HTMLElement)) {
      throw new Error("the waiting draft is not drawn as a notice");
    }
    expect(notice.className).toContain("callout-ai");
    expect(notice.textContent).toContain(
      "An agent drafted “After the fleet demo” for review.",
    );

    await user.click(
      within(notice).getByRole("button", {
        name: en["compose.waitingDraftOpen"],
      }),
    );

    expect(
      await screen.findByDisplayValue("After the fleet demo"),
    ).toBeTruthy();
    expect(
      await screen.findByText(en["compose.savedDraftByAgent"]),
    ).toBeTruthy();
  });

  it("appears when an agent leaves one after the page loaded", async () => {
    let saved = false;
    mount("overview", view, [], {
      "GET /mail-drafts": () =>
        saved
          ? jsonResponse(AGENT_DRAFT)
          : jsonResponse({ code: "not_found" }, 404),
    });
    await screen.findByRole("heading", { name: view.contact.full_name });
    expect(screen.queryByText(en["compose.waitingDraftTitle"])).toBeNull();

    saved = true;
    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });

    expect(
      await screen.findByText(en["compose.waitingDraftTitle"]),
    ).toBeTruthy();
  });

  it("is absent when the reader keeps no draft here", async () => {
    mount("overview", view, [], {
      "GET /mail-drafts": () => jsonResponse({ code: "not_found" }, 404),
    });
    await screen.findByRole("heading", { name: view.contact.full_name });
    expect(screen.queryByText(en["compose.waitingDraftTitle"])).toBeNull();
  });
});
