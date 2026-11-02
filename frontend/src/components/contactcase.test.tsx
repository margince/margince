// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
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
import {
  defaultSelection,
  isConnected,
  pairKeep,
  pairProblem,
} from "./contactcase.selection";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
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

// Answers 204 only to the one body the test expects, so a wrong request fails
// the press instead of passing it.
function expectBody(expected: unknown) {
  return (body: unknown) =>
    JSON.stringify(body) === JSON.stringify(expected)
      ? noContent()
      : jsonResponse({ title: "Unexpected body", status: 422 }, 422);
}

const TOO_LARGE = {
  too_large: true,
  can_merge: false,
  merge_mode: "none",
  reason: "too_large",
} as const;

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
    expect(member(/M\. Kovac/)).not.toBeChecked();
    // Dana is linked to Cleo, and was marked as a different person from Ana.
    expect(member(/Mira Kovacs/)).not.toBeChecked();
    expect(member(/Mira Kovacs/)).toBeDisabled();
    expect(
      screen.getByText("Marked as a different person from Mira Kovac."),
    ).toBeInTheDocument();
  });

  it("ticks one pair and never a chain of pairs", () => {
    const one = contactCase();
    // Ana–Ben and Ben–Cleo are open, but nothing pairs Ana with Cleo.
    expect(defaultSelection(one, ANA)).toEqual([ANA, BEN]);
    // Ben's strongest open pair is Ana (0.92), not Cleo (0.81).
    expect(defaultSelection(one, BEN)).toEqual([BEN, ANA]);
    // Dana's only open pair is Cleo; the Ana–Dana pair was marked different.
    expect(defaultSelection(one, DANA)).toEqual([DANA, CLEO]);
    expect(isConnected(one, [ANA, CLEO])).toBe(false);
    expect(isConnected(one, [ANA, BEN, CLEO])).toBe(true);
  });

  it("ticks only the start when it has no open pair", () => {
    const one = contactCase({
      edges: contactCase().edges.filter((edge) => edge.pair_id === "p4"),
    });
    expect(defaultSelection(one, ANA)).toEqual([ANA]);
  });
});

describe("a reader who may change one end of a pair", () => {
  // The reader may change Ana's record only, so Ben's must survive.
  const pairCase = () => {
    const one = contactCase({ merge_mode: "pair" });
    return {
      ...one,
      edges: one.edges.map((edge) =>
        edge.pair_id === "p1"
          ? { ...edge, can_dismiss: false, keep_id: BEN }
          : { ...edge, can_dismiss: false, can_merge: false },
      ),
    };
  };

  it("ticks the pair, keeps the colleague's record and its owner", async () => {
    const one = pairCase();
    const previews: unknown[] = [];
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => jsonResponse(one),
        [`POST ${CASE_PATH}/preview`]: (body) => {
          previews.push(body);
          return jsonResponse(previewOutcome({ mode: "pair" }));
        },
      },
      {},
    );
    const user = userEvent.setup();
    mount();
    expect(
      await screen.findByRole("checkbox", { name: /Mira Kovac\b/ }),
    ).toBeChecked();
    expect(member(/Mira Kovač/)).toBeChecked();
    expect(member(/M\. Kovac/)).not.toBeChecked();
    expect(screen.getByRole("radio", { name: /Mira Kovač/ })).toBeChecked();
    expect(screen.getByRole("radio", { name: /Mira Kovac$/ })).toBeDisabled();
    const owner = screen.getByRole("combobox", { name: /Owner/ });
    expect(owner).toHaveTextContent("Ben Seller");
    await user.click(owner);
    const offered = within(await screen.findByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual(["Ben Seller"]);
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "Preview merge" }));
    await waitFor(() =>
      expect(previews).toEqual([
        {
          revision: "rev-1",
          member_ids: [ANA, BEN],
          survivor_id: BEN,
          owner_id: one.members[1]?.owner_id,
        },
      ]),
    );
  });

  it("starts on an offered pair when the review opens on another member", () => {
    expect(defaultSelection(pairCase(), DANA)).toEqual([ANA, BEN]);
  });

  it("keeps the selection to the two ends of one offered pair", async () => {
    const one = pairCase();
    const toCleo = {
      ...one.edges[0],
      pair_id: "p5",
      right_id: CLEO,
      keep_id: CLEO,
    };
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse({ ...one, edges: [...one.edges, toCleo] }),
      },
      {},
    );
    const user = userEvent.setup();
    mount();
    const cleo = await screen.findByRole("checkbox", { name: /M\. Kovac/ });
    expect(cleo).toBeDisabled();

    await user.click(member(/Mira Kovač/));
    expect(cleo).toBeEnabled();
    // Dana is joined to Ana by a dismissed pair only.
    expect(member(/Mira Kovacs/)).toBeDisabled();
    await user.click(cleo);
    expect(member(/Mira Kovač/)).toBeDisabled();
    expect(screen.getByRole("radio", { name: /M\. Kovac/ })).toBeChecked();
    expect(screen.getByRole("combobox", { name: /Owner/ })).toHaveTextContent(
      "Cleo Reader",
    );
  });

  it("offers no survivor for a pair the server will not merge", () => {
    // Ben and Cleo are joined by an open pair the reader may not merge.
    expect(pairKeep(pairCase(), [BEN, CLEO])).toBeUndefined();
    expect(pairProblem(pairCase(), [BEN, CLEO])).toBe("contactCase.pairOnly");
    expect(pairProblem(pairCase(), [ANA, BEN, CLEO])).toBe(
      "contactCase.pairOnly",
    );
    expect(pairProblem(pairCase(), [ANA, BEN])).toBeNull();
  });

  it("says why and offers no preview when no pair can keep a live owner", async () => {
    // Ben's owner left, so Ben's record could keep no owner.
    const one = pairCase();
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse({
            ...one,
            owners: one.owners.filter(
              (owner) => !owner.member_ids.includes(BEN),
            ),
          }),
      },
      {},
    );
    mount();
    expect(
      await screen.findByText(
        "You can merge only one pair of these contacts, one of them yours. Tick such a pair.",
      ),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Preview merge" }),
    ).toBeDisabled();
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
    expect(offered).toEqual(["Ana Owner", "Ben Seller"]);
    await user.keyboard("{Escape}");

    await user.click(member(/M\. Kovac/));
    await user.click(owner);
    const widened = within(await screen.findByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(widened).toEqual(["Ana Owner", "Ben Seller", "Cleo Reader"]);
  });

  it("follows the kept record until the reviewer picks an owner", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    const user = userEvent.setup();
    mount();
    const owner = await screen.findByRole("combobox", { name: /Owner/ });
    await user.click(member(/M\. Kovac/));
    await user.click(screen.getByRole("radio", { name: /Mira Kovač/ }));
    expect(owner).toHaveTextContent("Ben Seller");

    await user.click(owner);
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "Cleo Reader",
      }),
    );
    await user.click(screen.getByRole("radio", { name: /Mira Kovac$/ }));
    expect(owner).toHaveTextContent("Cleo Reader");
  });
});

describe("members that share a name", () => {
  it("are told apart by their email", async () => {
    const twins = contactCase();
    const members = twins.members.map((one) => ({
      ...one,
      name: "Alex Example",
    }));
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => jsonResponse({ ...twins, members }),
      },
      {},
    );
    mount();
    expect(
      await screen.findByRole("radio", {
        name: /Alex Example \(m\.kovac@northwind\.de\)/,
      }),
    ).toBeInTheDocument();
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
      await screen.findByRole("checkbox", { name: /M\. Kovac/ }),
    );
    await user.click(screen.getByRole("button", { name: "Preview merge" }));
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
  it("says a colleague settled it when the case is gone after a case change", async () => {
    let reads = 0;
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => {
          reads += 1;
          return reads === 1
            ? jsonResponse(contactCase())
            : jsonResponse({ title: "Not found", status: 404 }, 404);
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
        "A colleague already settled these duplicates. This contact remains.",
      ),
    ).toBeVisible();
    expect(
      screen.queryByText("This contact has no open duplicates."),
    ).toBeNull();
  });

  it("does not credit a colleague with a pair the reviewer merged here", async () => {
    let reads = 0;
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => {
          reads += 1;
          return reads === 1
            ? jsonResponse(contactCase(TOO_LARGE))
            : jsonResponse({ title: "Not found", status: 404 }, 404);
        },
        "POST /dedupe/candidates/p1/disposition": noContent,
      },
      {},
    );
    const user = userEvent.setup();
    mountWithToasts();
    await user.click(
      await screen.findByRole("button", {
        name: "Merge these two: Mira Kovac and Mira Kovač",
      }),
    );
    await user.click(screen.getByRole("button", { name: "Keep Mira Kovač" }));
    expect(
      await screen.findByText("This contact has no open duplicates."),
    ).toBeVisible();
    expect(screen.queryByText(/A colleague already settled/)).toBeNull();
  });

  it("says the contact has no duplicates when it never had a case", async () => {
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse({ title: "Not found", status: 404 }, 404),
      },
      {},
    );
    mount();
    expect(
      await screen.findByText("This contact has no open duplicates."),
    ).toBeVisible();
  });

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

  it("merges a too-large case's pair into the kept contact, and stales both ends", async () => {
    const posted: unknown[] = [];
    const merge = expectBody({ disposition: "merge", winner_id: BEN });
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase(TOO_LARGE)),
        "POST /dedupe/candidates/p1/disposition": (body) => {
          posted.push(body);
          return merge(body);
        },
      },
      {},
    );
    const invalidated = vi.spyOn(QueryClient.prototype, "invalidateQueries");
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
    await waitFor(() => expect(posted).toHaveLength(1));
    const keys = () =>
      invalidated.mock.calls.map(([filters]) =>
        JSON.stringify(filters?.queryKey),
      );
    await waitFor(() =>
      expect(keys()).toEqual(
        expect.arrayContaining([
          JSON.stringify(["contact", ANA]),
          JSON.stringify(["contact", BEN]),
          JSON.stringify(["contacts"]),
          JSON.stringify(["worklist"]),
        ]),
      ),
    );
    expect(screen.queryByText("The pair was not saved. Retry.")).toBeNull();
  });

  it("offers a pair the reader may dismiss but not merge only the dismissal", async () => {
    const large = contactCase(TOO_LARGE);
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse({
            ...large,
            edges: large.edges.map((edge) => ({ ...edge, can_merge: false })),
          }),
      },
      {},
    );
    mountWithToasts();
    expect(
      await screen.findByRole("button", {
        name: "Not the same person: Mira Kovac and Mira Kovač",
      }),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: /Merge these two/ }),
    ).toBeNull();
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

  it("offers no pair verbs on a pair the server says the reader may not decide", async () => {
    const refused = contactCase();
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse({
            ...refused,
            can_merge: false,
            merge_mode: "none",
            reason: "no_write_authority",
            edges: refused.edges.map((edge) => ({
              ...edge,
              can_dismiss: false,
            })),
          }),
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
