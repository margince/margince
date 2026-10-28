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
import { ToastProvider, ToastRegion } from "../design-system/toast";
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

function mountWithToasts() {
  return render(
    <StoryProviders>
      <ToastProvider>
        <ContactCaseReview contactId={ANA} open onClose={() => undefined} />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
}

const noContent = () => new Response(null, { status: 204 });

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

describe("deciding one pair inside the review", () => {
  it("posts not_a_duplicate for that pair and reads the case again", async () => {
    let reads = 0;
    const posted: unknown[] = [];
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => {
          reads += 1;
          return jsonResponse(contactCase({ revision: `rev-${reads}` }));
        },
        "POST /dedupe/candidates/p1/disposition": (body) => {
          posted.push(body);
          return noContent();
        },
      },
      {},
    );
    const user = userEvent.setup();
    mountWithToasts();
    await user.click(
      await screen.findByRole("button", {
        name: "Not the same person: Mira Kovac and Mira Kovač",
      }),
    );
    await waitFor(() => expect(reads).toBe(2));
    expect(posted).toEqual([{ disposition: "not_a_duplicate" }]);
  });

  it("offers a too-large case's pairs a merge that names the kept contact", async () => {
    const posted: unknown[] = [];
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse(
            contactCase({
              too_large: true,
              can_merge: false,
              merge_mode: "none",
              reason: "too_large",
            }),
          ),
        "POST /dedupe/candidates/p1/disposition": (body) => {
          posted.push(body);
          return noContent();
        },
      },
      {},
    );
    const user = userEvent.setup();
    mountWithToasts();
    expect(
      await screen.findByText(/Decide its pairs one at a time below\./),
    ).toBeVisible();
    await user.click(
      screen.getByRole("button", {
        name: "Merge these two: Mira Kovac and Mira Kovač",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Keep Mira Kovač" }));
    await waitFor(() =>
      expect(posted).toEqual([{ disposition: "merge", winner_id: BEN }]),
    );
  });

  it("re-opens a pair marked as different people", async () => {
    const undone = vi.fn(noContent);
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()),
        "POST /dedupe/candidates/p4/undo": undone,
      },
      {},
    );
    const user = userEvent.setup();
    mountWithToasts();
    await user.click(
      await screen.findByRole("button", {
        name: "Undo: Mira Kovac and Mira Kovacs",
      }),
    );
    await waitFor(() => expect(undone).toHaveBeenCalledTimes(1));
  });

  it("says who can decide when the server refuses the reader", async () => {
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()),
        "POST /dedupe/candidates/p2/disposition": () =>
          jsonResponse(
            { title: "Forbidden", status: 403, code: "permission_denied" },
            403,
          ),
      },
      {},
    );
    const user = userEvent.setup();
    mountWithToasts();
    await user.click(
      await screen.findByRole("button", {
        name: "Not the same person: Mira Kovač and M. Kovac",
      }),
    );
    expect(
      await screen.findByText(
        "You cannot decide this pair. Ask an owner of these contacts or an administrator.",
      ),
    ).toBeVisible();
    expect(screen.queryByText(/Retry\./)).toBeNull();
  });

  it("offers no pair verbs to a reader who may not merge", async () => {
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
    mountWithToasts();
    await screen.findByText(/an administrator can turn on shared merge/);
    expect(
      screen.queryByRole("button", { name: /Not the same person/ }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: /^Undo/ })).toBeNull();
  });
});
