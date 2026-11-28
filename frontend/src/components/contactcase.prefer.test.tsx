// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../i18n/en";
import {
  ANA,
  BEN,
  CLEO,
  contactCase,
  previewOutcome,
} from "../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { ContactCaseReview } from "./contactcase";
import { preferChoices, preferFor } from "./contactcase.selection";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const CASE_PATH = `/dedupe/contact-cases/${ANA}`;

describe("which values the reviewer chooses between", () => {
  it("offers only the fields the ticked members disagree on, the kept record first", () => {
    const choices = preferChoices(contactCase(), [ANA, BEN], BEN);
    expect(choices.map((choice) => choice.field)).toEqual(["name", "email"]);
    expect(choices[1]?.options).toEqual([
      { member: BEN, value: "m.kovac@northwind.de" },
      { member: ANA, value: "mira@northwind.de" },
    ]);
  });

  it("sends a choice only while its member is ticked and the field still differs", () => {
    const one = contactCase();
    expect(preferFor(one, [ANA, BEN], ANA, { email: BEN })).toEqual({
      email: BEN,
    });
    expect(preferFor(one, [ANA, CLEO], ANA, { email: BEN })).toBeUndefined();
    expect(preferFor(one, [ANA, BEN], ANA, {})).toBeUndefined();
  });
});

describe("the review form", () => {
  it("sends the preferred email with the preview", async () => {
    const previews: unknown[] = [];
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()),
        [`POST ${CASE_PATH}/preview`]: (body) => {
          previews.push(body);
          return jsonResponse(previewOutcome());
        },
      },
      {},
    );
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <ContactCaseReview contactId={ANA} open onClose={() => undefined} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("checkbox", { name: /Mira Kovač/ }),
    );
    const email = screen.getByRole("combobox", {
      name: englishCopy["contactCase.prefer.email"],
    });
    expect(email).toHaveTextContent("mira@northwind.de");
    await user.click(email);
    await user.click(
      await screen.findByRole("option", { name: "m.kovac@northwind.de" }),
    );

    await user.click(
      screen.getByRole("button", {
        name: englishCopy["contactCase.previewVerb"],
      }),
    );
    await waitFor(() =>
      expect(previews).toEqual([
        expect.objectContaining({ prefer: { email: BEN } }),
      ]),
    );
  });
});
