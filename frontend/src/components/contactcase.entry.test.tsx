// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { routeHash } from "../lib/router";
import {
  ANA,
  BEN,
  contactCase,
  previewOutcome,
} from "../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { ContactCaseEntry } from "./contactcase.entry";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

const CASE_PATH = `/dedupe/contact-cases/${ANA}`;

function mount(followSurvivor = false) {
  return render(
    <StoryProviders>
      <ToastProvider>
        <ContactCaseEntry contactId={ANA} followSurvivor={followSurvivor} />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
}

describe("the door to a contact's duplicate case", () => {
  it("counts the other members of a case listed in full", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    mount();
    expect(
      await screen.findByRole("button", { name: "Possible duplicates (3)" }),
    ).toBeVisible();
  });

  it("reads the count as a floor when the case is too large to list", async () => {
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse(contactCase({ too_large: true })),
      },
      {},
    );
    mount();
    expect(
      await screen.findByRole("button", { name: "Possible duplicates (4+)" }),
    ).toBeVisible();
  });

  it("opens the contact that remained, and says the merge happened", async () => {
    let merged = false;
    const outcome = previewOutcome({
      survivor_id: BEN,
      merged_ids: [ANA],
      committed: true,
    });
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          merged ? jsonResponse(null) : jsonResponse(contactCase()),
        [`POST ${CASE_PATH}/preview`]: () =>
          jsonResponse({ ...outcome, committed: false }),
        [`POST ${CASE_PATH}/merge`]: () => {
          merged = true;
          return jsonResponse(outcome);
        },
      },
      {},
    );
    const user = userEvent.setup();
    mount(true);
    await user.click(await screen.findByTestId("contact-case-entry"));
    await user.click(
      await screen.findByRole("checkbox", { name: /Mira Kovač/ }),
    );
    await user.click(
      screen.getByRole("radio", { name: "Keep details of Mira Kovač" }),
    );
    await user.click(screen.getByRole("button", { name: "Preview merge" }));
    await user.click(
      await screen.findByRole("button", { name: "Merge 2 contacts" }),
    );
    await waitFor(() =>
      expect(globalThis.location.hash).toBe(
        routeHash({ screen: "contacts", id: BEN }),
      ),
    );
    expect(await screen.findByText("Contacts merged")).toBeVisible();
  });
});
