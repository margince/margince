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
import { en } from "../i18n/en";
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

function mount({ chosen }: { chosen?: boolean } = {}) {
  return render(
    <StoryProviders>
      <ContactCaseReview
        contactId={ANA}
        chosen={chosen}
        open
        onClose={() => undefined}
      />
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
    const user = userEvent.setup();
    mount();
    expect(
      await screen.findByRole("checkbox", { name: /Mira Kovac\b/ }),
    ).toBeChecked();
    // Dana is linked to Cleo, and was marked as a different person from Ana.
    expect(member(/Mira Kovacs/)).not.toBeChecked();
    expect(member(/Mira Kovacs/)).toBeDisabled();
    expect(
      screen.getByText("Marked as a different person from Mira Kovac."),
    ).toBeInTheDocument();

    await user.click(member(/Mira Kovač/));
    await user.click(member(/M\. Kovac/));
    expect(member(/Mira Kovacs/)).not.toBeChecked();
    expect(member(/Mira Kovacs/)).toBeDisabled();
  });

  it("ticks only the contact the review opened on, never a second one", () => {
    const one = contactCase();
    expect(defaultSelection(one, ANA)).toEqual([ANA]);
    expect(defaultSelection(one, BEN)).toEqual([BEN]);
    // Dana's Ana pair was marked different, but her Cleo pair is open.
    expect(defaultSelection(one, DANA)).toEqual([DANA]);
  });

  it("ticks nobody when the review was opened on no contact", () => {
    expect(defaultSelection(contactCase(), undefined)).toEqual([]);
  });

  it("ticks nobody when the start's only pair is marked as different people", () => {
    const one = contactCase({
      edges: contactCase().edges.filter((edge) => edge.pair_id === "p4"),
    });
    expect(defaultSelection(one, ANA)).toEqual([]);
  });

  it("joins ticked members only through open pairs among them", () => {
    const one = contactCase();
    // Ana–Ben and Ben–Cleo are open, but nothing pairs Ana with Cleo.
    expect(isConnected(one, [ANA, CLEO])).toBe(false);
    expect(isConnected(one, [ANA, BEN, CLEO])).toBe(true);
  });

  it("starts with nobody ticked and no preview when the server picked the contact", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    mount({ chosen: false });
    const preview = await screen.findByRole("button", {
      name: en["contactCase.previewVerb"],
    });
    expect(preview).toBeDisabled();
    for (const box of screen.getAllByRole("checkbox")) {
      expect(box).not.toBeChecked();
    }
  });

  it("starts on the opened contact alone and previews once the reader ticks a duplicate", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    const user = userEvent.setup();
    mount();
    const preview = await screen.findByRole("button", {
      name: en["contactCase.previewVerb"],
    });
    expect(screen.getAllByRole("checkbox", { checked: true })).toEqual([
      member(/Mira Kovac\b/),
    ]);
    expect(preview).toBeDisabled();

    await user.click(member(/Mira Kovač/));
    expect(preview).toBeEnabled();
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

  it("keeps the colleague's record and its owner once the reader ticks the pair", async () => {
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
    expect(member(/Mira Kovač/)).not.toBeChecked();
    expect(
      screen.getByRole("button", { name: en["contactCase.previewVerb"] }),
    ).toBeDisabled();

    await user.click(member(/Mira Kovač/));
    expect(member(/M\. Kovac/)).not.toBeChecked();
    expect(screen.getByRole("radio", { name: /Mira Kovač/ })).toBeChecked();
    expect(screen.queryByRole("radio", { name: /Mira Kovac$/ })).toBeNull();
    const owner = screen.getByRole("combobox", { name: /Owner/ });
    expect(owner).toHaveTextContent("Ben Seller");
    await user.click(owner);
    const offered = within(await screen.findByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual(["Ben Seller"]);
    await user.keyboard("{Escape}");

    await user.click(
      screen.getByRole("button", { name: en["contactCase.previewVerb"] }),
    );
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

  it("starts ticked only on a member of a pair the reader may merge", () => {
    expect(defaultSelection(pairCase(), ANA)).toEqual([ANA]);
    expect(defaultSelection(pairCase(), BEN)).toEqual([BEN]);
    // Cleo's only open pair is one the reader may not merge.
    expect(defaultSelection(pairCase(), CLEO)).toEqual([]);
    expect(defaultSelection(pairCase(), DANA)).toEqual([]);
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
    expect(cleo).toBeEnabled();

    await user.click(member(/Mira Kovač/));
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
      await screen.findByRole("button", {
        name: en["contactCase.previewVerb"],
      }),
    ).toBeDisabled();
    for (const box of screen.getAllByRole("checkbox")) {
      expect(box).not.toBeChecked();
      expect(box).toBeDisabled();
    }
    // The pair rule is the reason, not a request to tick what cannot be ticked.
    expect(screen.getByText(en["contactCase.pairOnly"])).toBeInTheDocument();
    expect(screen.queryByText(en["contactCase.tooFew"])).toBeNull();
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
    await user.click(member(/Mira Kovač/));
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

  it("marks an owner who has not signed in yet as invited", async () => {
    const base = contactCase();
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () =>
          jsonResponse({
            ...base,
            owners: base.owners.map((one) =>
              one.name === "Ben Seller" ? { ...one, status: "invited" } : one,
            ),
          }),
      },
      {},
    );
    const user = userEvent.setup();
    mount();
    const owner = await screen.findByRole("combobox", { name: /Owner/ });
    await user.click(member(/Mira Kovač/));
    await user.click(owner);
    const offered = within(await screen.findByRole("listbox"))
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual(["Ana Owner", "Ben Seller (invited)"]);
  });

  it("follows the kept record until the reviewer picks an owner", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    const user = userEvent.setup();
    mount();
    const owner = await screen.findByRole("combobox", { name: /Owner/ });
    await user.click(member(/Mira Kovač/));
    await user.click(member(/M\. Kovac/));
    await user.click(screen.getByRole("radio", { name: /Mira Kovač/ }));
    expect(owner).toHaveTextContent("Ben Seller");

    await user.click(owner);
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "Cleo Reader",
      }),
    );
    await user.click(screen.getByRole("radio", { name: /M\. Kovac/ }));
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
    const user = userEvent.setup();
    const boxes = await screen.findAllByRole("checkbox");
    await user.click(boxes[1]);
    expect(
      await screen.findByRole("radio", {
        name: /Alex Example \(m\.kovac@northwind\.de\) stays/,
      }),
    ).toBeInTheDocument();
  });
});

describe("the two choices", () => {
  it("offers a stays radio only for ticked contacts, in a group with its own question", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    const user = userEvent.setup();
    mount();
    await screen.findByRole("checkbox", { name: /Mira Kovac\b/ });
    expect(screen.getAllByRole("radio")).toHaveLength(1);
    const group = screen.getByRole("group", {
      name: en["contactCase.survivor.legend"],
    });
    expect(
      within(group).getByRole("radio", { name: "Mira Kovac stays" }),
    ).toBeChecked();
    expect(
      within(group).getByText(en["contactCase.survivor.hint"]),
    ).toBeVisible();
    expect(screen.queryByText(/Keep this record/)).toBeNull();

    await user.click(member(/Mira Kovač/));
    expect(screen.getAllByRole("radio")).toHaveLength(2);
    await user.click(member(/Mira Kovac\b/));
    expect(screen.getAllByRole("radio")).toHaveLength(1);
    expect(
      screen.getByRole("radio", { name: "Mira Kovač stays" }),
    ).toBeChecked();
  });

  it("previews and merges the chosen survivor and exactly the ticked contacts", async () => {
    type Decision = {
      survivor_id: string;
      member_ids: string[];
      owner_id: string;
    };
    const previews: Decision[] = [];
    const merges: Decision[] = [];
    const echo = (body: unknown) => {
      const sent = body as Decision;
      return previewOutcome({
        survivor_id: sent.survivor_id,
        merged_ids: sent.member_ids.filter((id) => id !== sent.survivor_id),
        owner_id: sent.owner_id,
      });
    };
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()),
        [`POST ${CASE_PATH}/preview`]: (body) => {
          previews.push(body as Decision);
          return jsonResponse(echo(body));
        },
        [`POST ${CASE_PATH}/merge`]: (body) => {
          merges.push(body as Decision);
          return jsonResponse({ ...echo(body), committed: true });
        },
      },
      {},
    );
    const user = userEvent.setup();
    mount();
    await user.click(
      await screen.findByRole("checkbox", { name: /Mira Kovač/ }),
    );
    await user.click(member(/M\. Kovac/));
    await user.click(member(/Mira Kovac\b/));
    await user.click(member(/M\. Kovac/));
    await user.click(member(/M\. Kovac/));
    await user.click(screen.getByRole("radio", { name: "M. Kovac stays" }));
    await user.click(
      screen.getByRole("button", { name: en["contactCase.previewVerb"] }),
    );
    const preview = await screen.findByTestId("contact-case-preview");
    expect(previews[0]).toMatchObject({
      survivor_id: CLEO,
      member_ids: [BEN, CLEO],
    });
    expect(
      within(preview).getByText(/^M\. Kovac stays, owned by/),
    ).toBeVisible();
    expect(within(preview).getByText("Mira Kovač")).toBeVisible();
    expect(within(preview).queryByText("M. Kovac")).toBeNull();
    expect(within(preview).queryByText("Mira Kovac")).toBeNull();

    await user.click(screen.getByTestId("contact-case-merge"));
    await vi.waitFor(() => expect(merges).toHaveLength(1));
    expect(merges[0].survivor_id).toBe(CLEO);
    expect(merges[0].member_ids).toEqual([BEN, CLEO]);
  });

  it("makes the first contact ticked from a card the survivor", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    const user = userEvent.setup();
    mount({ chosen: false });
    await screen.findByRole("button", { name: en["contactCase.previewVerb"] });
    await user.click(member(/Mira Kovač/));
    expect(
      screen.getByRole("radio", { name: "Mira Kovač stays" }),
    ).toBeChecked();
    await user.click(member(/M\. Kovac/));
    expect(
      screen.getByRole("radio", { name: "Mira Kovač stays" }),
    ).toBeChecked();
  });

  it("says what Not the same person does to the pair and to unticked contacts", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(contactCase()) },
      {},
    );
    mount();
    expect(
      await screen.findByText(/stops only that pair from being suggested/),
    ).toBeVisible();
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
      await screen.findByRole("checkbox", { name: /Mira Kovač/ }),
    );
    await user.click(member(/M\. Kovac/));
    await user.click(
      screen.getByRole("button", { name: en["contactCase.previewVerb"] }),
    );
    const preview = await screen.findByTestId("contact-case-preview");
    expect(within(preview).getByText("Cleo Reader, can view")).toBeVisible();
    expect(
      within(preview).getByText("Ben Seller: account is inactive"),
    ).toBeVisible();
    expect(
      within(preview).getByText(en["contactCase.preview.final"]),
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
      await screen.findByRole("checkbox", { name: /Mira Kovač/ }),
    );
    await user.click(
      screen.getByRole("button", { name: en["contactCase.previewVerb"] }),
    );
    expect(await screen.findByText(en["contactCase.stale"])).toBeVisible();
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
            : new Response(null, { status: 204 });
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
      await screen.findByRole("checkbox", { name: /Mira Kovač/ }),
    );
    await user.click(
      screen.getByRole("button", { name: en["contactCase.previewVerb"] }),
    );
    expect(
      await screen.findByText(en["contactCase.settledElsewhere"]),
    ).toBeVisible();
    expect(screen.queryByText(en["contactCase.none"])).toBeNull();
  });

  it("does not credit a colleague with a pair the reviewer merged here", async () => {
    let reads = 0;
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => {
          reads += 1;
          return reads === 1
            ? jsonResponse(contactCase(TOO_LARGE))
            : new Response(null, { status: 204 });
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
    expect(await screen.findByText(en["contactCase.none"])).toBeVisible();
    expect(screen.queryByText(/A colleague already settled/)).toBeNull();
  });

  it("says the contact has no duplicates when it never had a case", async () => {
    stubWithSession(
      {
        [`GET ${CASE_PATH}`]: () => new Response(null, { status: 204 }),
      },
      {},
    );
    mount();
    expect(await screen.findByText(en["contactCase.none"])).toBeVisible();
  });

  it("says who merged it, and into which contact", async () => {
    stubWithSession(
      { [`GET ${CASE_PATH}`]: () => jsonResponse(resolvedCase()) },
      {},
    );
    mount();
    expect(await screen.findByText("Merged by Ben Seller")).toBeVisible();
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["contactCase.previewVerb"] }),
    ).toBeNull();
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
    expect(
      screen.queryByRole("button", { name: en["contactCase.previewVerb"] }),
    ).toBeNull();
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
    expect(screen.queryByText(en["contactCase.pair.failed"])).toBeNull();
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
      await screen.findByText(en["contactCase.pair.refused"]),
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
