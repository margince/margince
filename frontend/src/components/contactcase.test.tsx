// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ANA,
  BEN,
  CLEO,
  contactCase,
  DANA,
  previewOutcome,
  resolvedCase,
} from "../testing/contactcase.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { ContactCaseReview } from "./contactcase";
import { defaultSelection, isConnected } from "./contactcase.selection";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CASE_PATH = `/dedupe/contact-cases/${ANA}`;

function mount() {
  return render(
    <StoryProviders>
      <ContactCaseReview contactId={ANA} open onClose={() => undefined} />
    </StoryProviders>,
  );
}

function member(name: RegExp) {
  return screen.getByRole("checkbox", { name });
}

describe("which members the review ticks", () => {
  it("never ticks a member across a pair someone marked as different people", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    mount();
    expect(
      await screen.findByRole("checkbox", { name: /Mira Kovac\b/ }),
    ).toBeChecked();
    expect(member(/Mira Kovač/)).toBeChecked();
    expect(member(/M\. Kovac/)).toBeChecked();
    // Dana is linked to Cleo, and was marked as a different person from Ana.
    expect(member(/Mira Kovacs/)).not.toBeChecked();
    expect(member(/Mira Kovacs/)).toBeDisabled();
    expect(
      screen.getByText("Marked as a different person from Mira Kovac."),
    ).toBeInTheDocument();
  });

  it("walks open pairs only, and keeps the walk connected", () => {
    const one = contactCase();
    expect(defaultSelection(one, ANA)).toEqual([ANA, BEN, CLEO]);
    // From Dana, Cleo and Ben come in; Ana was marked as a different person.
    expect(defaultSelection(one, DANA)).toEqual([DANA, CLEO, BEN]);
    expect(isConnected(one, [ANA, CLEO])).toBe(false);
    expect(isConnected(one, [ANA, BEN, CLEO])).toBe(true);
  });
});

describe("who may own the remaining contact", () => {
  it("offers only the owners of the ticked members", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    const user = userEvent.setup();
    mount();
    const owner = await screen.findByRole("combobox", { name: /Owner/ });
    // The kept record's own owner is the start.
    expect(owner).toHaveTextContent("Ana Owner");
    await user.click(owner);
    const offered = within(await screen.findByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual(["Ana Owner", "Ben Seller", "Cleo Reader"]);
    await user.keyboard("{Escape}");

    await user.click(member(/M\. Kovac/));
    await user.click(owner);
    const left = within(await screen.findByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(left).toEqual(["Ana Owner", "Ben Seller"]);
  });
});

describe("the preview", () => {
  it("says who keeps access, who gets nothing and that it is final", async () => {
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
    mount();
    await user.click(
      await screen.findByRole("button", { name: "Preview merge" }),
    );
    const preview = await screen.findByTestId("contact-case-preview");
    expect(within(preview).getByText("Cleo Reader, can view")).toBeVisible();
    expect(
      within(preview).getByText("Ben Seller: account is inactive"),
    ).toBeVisible();
    expect(
      within(preview).getByText("A merge cannot be undone."),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Merge 3 contacts" }),
    ).toBeVisible();
    expect(previews).toEqual([
      {
        revision: "rev-1",
        member_ids: [ANA, BEN, CLEO],
        survivor_id: ANA,
        owner_id: contactCase().owners[0]?.user_id,
      },
    ]);
  });

  it("reads the case again when it changed under the review", async () => {
    let reads = 0;
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => {
          reads += 1;
          return jsonResponse(contactCase({ revision: `rev-${reads}` }));
        },
        [`POST ${CASE_PATH}/preview`]: () =>
          jsonResponse(
            { title: "Conflict", status: 409, code: "case_changed" },
            409,
          ),
      },
      {},
    );
    const user = userEvent.setup();
    mount();
    await user.click(
      await screen.findByRole("button", { name: "Preview merge" }),
    );
    expect(
      await screen.findByText(
        "The duplicates changed while you reviewed them. Check the selection again.",
      ),
    ).toBeVisible();
    await waitFor(() => expect(reads).toBe(2));
  });
});

describe("a case already settled", () => {
  it("says who merged it, and into which contact", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(resolvedCase()) },
      {},
    );
    mount();
    expect(await screen.findByText("Merged by Ben Seller")).toBeVisible();
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.queryByRole("button", { name: "Preview merge" })).toBeNull();
  });

  it("explains why a reader without authority cannot merge", async () => {
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse(
            contactCase({
              can_merge: false,
              merge_mode: "none",
              reason: "no_write_authority",
            }),
          ),
      },
      {},
    );
    mount();
    expect(
      await screen.findByText(/an administrator can turn on shared merge/),
    ).toBeVisible();
    expect(member(/Mira Kovač/)).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Preview merge" })).toBeNull();
  });
});
