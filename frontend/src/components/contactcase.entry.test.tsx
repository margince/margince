// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ANA, contactCase } from "../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { ContactCaseEntry } from "./contactcase.entry";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CASE_PATH = `/dedupe/contact-cases/${ANA}`;

function mount() {
  return render(
    <StoryProviders>
      <ContactCaseEntry contactId={ANA} />
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
});
