/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { en } from "../i18n/en";
import type { FilterVocabulary } from "./filterdata";
import { FiltersScreen } from "./filters";
import { type FiltersServer, mountFilters, type Sent } from "./filters.testkit";

// A proposal lands as ordinary rows marked as proposed: these tests are about
// where it lands, what the reader may do with it before, during and after, and
// that every phrase it could not use is named back rather than lost.

const VOCAB: FilterVocabulary = {
  resource: "contact",
  fields: [
    {
      name: "full_name",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
    {
      name: "city",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
  ],
};

const ASKED = "contacts named Lee in Berlin";

const PROPOSAL = {
  resource: "contact",
  filter: {
    and: [
      { field: "full_name", op: "contains", value: "Lee" },
      { field: "city", op: "eq", value: "Berlin" },
    ],
  },
  unsupported: [],
};

const NOTHING_USABLE = {
  resource: "contact",
  filter: null,
  unsupported: [
    {
      phrase: "likely to buy",
      code: "not_expressible",
      reason: "No field records a prediction.",
    },
    {
      phrase: "in the gold tier",
      code: "unknown_field",
      reason: "not a field",
      field: "cf_tier",
    },
  ],
};

type Proposals = NonNullable<FiltersServer["proposals"]>;

function mount(proposals: Proposals = [{ body: PROPOSAL }]) {
  const mounted = mountFilters({
    vocabularies: { contact: VOCAB },
    preview: { match_count: 4 },
    proposals,
  });
  render(<FiltersScreen id="contacts" />, { wrapper: mounted.wrapper });
  return mounted;
}

/** A proposal answer held open until the test lets it through. */
function held(body: unknown) {
  let answer = () => {};
  const answered = new Promise<void>((resolve) => {
    answer = resolve;
  });
  return { entry: { body, answered }, answer: () => answer() };
}

afterEach(cleanup);

type User = ReturnType<typeof userEvent.setup>;

/** Describes the filter and sends it, opening the folded box when it is shut. */
async function ask(user: User, text = ASKED) {
  const folded = screen.queryByText(en["filters.describeChanges"]);
  if (folded && folded.closest("details")?.open !== true) {
    await user.click(folded);
  }
  const box = await screen.findByLabelText("Describe the contacts you want");
  await user.clear(box);
  await user.type(box, text);
  await user.click(screen.getByRole("button", { name: "Propose conditions" }));
}

/** One condition of the reader's own, written by hand. */
async function addOwn(user: User, value: string) {
  const adds = await screen.findAllByRole("button", {
    name: en["filters.addClause"],
  });
  await user.click(adds[0]);
  const inputs = screen.getAllByLabelText<HTMLInputElement>("Value");
  await user.type(inputs[inputs.length - 1], value);
}

const values = () =>
  screen
    .queryAllByLabelText<HTMLInputElement>("Value")
    .map((input) => input.value);

const firstField = () => screen.getAllByRole("combobox", { name: "Field" })[0];

const proposedRows = () =>
  document.querySelectorAll(".filter-clause[data-proposed]");

const barTitle = (count: number, text = ASKED) =>
  `Margince proposed ${count} ${count === 1 ? "condition" : "conditions"} from “${text}”.`;

const sentTo = (seen: readonly Sent[], path: string) =>
  seen.filter((sent) => sent.url.includes(path)).map((sent) => sent.body);

it("lands a proposal on the calm start as marked rows the reader can keep", async () => {
  const { seen } = mount();
  const user = userEvent.setup();

  await ask(user);

  expect(await screen.findByText(barTitle(2))).toBeTruthy();
  expect(values()).toEqual(["Lee", "Berlin"]);
  expect(proposedRows()).toHaveLength(2);
  expect(screen.getAllByText(en["filters.proposed"])).toHaveLength(2);
  expect(screen.getByRole("button", { name: "Keep all" })).toBeTruthy();
  expect(screen.getByRole("button", { name: en["common.undo"] })).toBeTruthy();
  // Nothing of the reader's own to replace.
  expect(
    screen.queryByRole("button", { name: "Replace my conditions" }),
  ).toBeNull();
  // The box the reader asked from is gone; they are in the first new row.
  expect(document.activeElement).toBe(firstField());
  expect(sentTo(seen, "/filters/propose")).toEqual([
    { resource: "contact", text: ASKED, locale: "en" },
  ]);
  // The count runs on the whole tree, proposals included, marks dropped.
  expect(await screen.findByText("4 contacts match")).toBeTruthy();
  const preview = sentTo(seen, "/filters/preview").at(-1) as {
    filter: unknown;
  };
  expect(preview.filter).toEqual(PROPOSAL.filter);

  await user.click(screen.getByRole("button", { name: "Keep all" }));

  expect(proposedRows()).toHaveLength(0);
  expect(screen.queryByText(barTitle(2))).toBeNull();
  expect(values()).toEqual(["Lee", "Berlin"]);
  // Keep all went with the bar; the reader is back at the first row.
  expect(document.activeElement).toBe(firstField());
});

it("undoes a proposal back to the calm start, and sends a description on Enter", async () => {
  mount();
  const user = userEvent.setup();

  await user.type(
    await screen.findByLabelText("Describe the contacts you want"),
    `${ASKED}{Enter}`,
  );
  expect(await screen.findByText(barTitle(2))).toBeTruthy();
  expect(await screen.findByText("4 contacts match")).toBeTruthy();

  await user.click(screen.getByRole("button", { name: en["common.undo"] }));

  expect(values()).toEqual([]);
  expect(
    screen.getByRole("heading", { name: "Build it condition by condition" }),
  ).toBeTruthy();
  expect(screen.queryByText("Matching records")).toBeNull();
  // The Undo that was pressed is gone with the bar; the calm start takes focus.
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: en["filters.addClause"] }),
  );
});

it("appends a proposal to the reader's own condition and replaces it only when asked", async () => {
  mount();
  const user = userEvent.setup();
  await addOwn(user, "Ann");

  await ask(user);

  expect(await screen.findByText(barTitle(2))).toBeTruthy();
  expect(values()).toEqual(["Ann", "Lee", "Berlin"]);
  // Asked from the folded box, which folds again once the rows have landed.
  expect(
    screen.getByText(en["filters.describeChanges"]).closest("details")?.open,
  ).toBe(false);

  await user.click(
    screen.getByRole("button", { name: "Replace my conditions" }),
  );
  expect(values()).toEqual(["Lee", "Berlin"]);
  expect(
    screen.queryByRole("button", { name: "Replace my conditions" }),
  ).toBeNull();
  // The button pressed is gone; the question still open about the rows is not.
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Keep all" }),
  );

  await user.click(screen.getByRole("button", { name: en["common.undo"] }));
  expect(values()).toEqual(["Ann"]);
  expect(proposedRows()).toHaveLength(0);
  expect(screen.queryByText(barTitle(2))).toBeNull();
  expect(document.activeElement).toBe(firstField());
});

it("lands as one group on a root that matches any, and a group joined the other way is the reader's", async () => {
  mount();
  const user = userEvent.setup();
  await addOwn(user, "Ann");
  await addOwn(user, "Bo");
  await user.click(
    screen.getByRole("button", {
      name: "and: match all of these. Press to match any.",
    }),
  );

  await ask(user);

  expect(await screen.findByText(barTitle(2))).toBeTruthy();
  const group = screen.getByRole("group", { name: "All of these" });
  expect(
    within(group)
      .getAllByLabelText<HTMLInputElement>("Value")
      .map((input) => input.value),
  ).toEqual(["Lee", "Berlin"]);
  expect(group.querySelectorAll("[data-proposed]")).toHaveLength(2);

  await user.click(
    within(group).getByRole("button", {
      name: "and: match all of these. Press to match any.",
    }),
  );

  // Joining it the other way rewrote the group, so nothing in it is a
  // proposal now and a second answer could not take it away.
  expect(proposedRows()).toHaveLength(0);
  expect(screen.queryByText(barTitle(2))).toBeNull();
  expect(values()).toEqual(["Ann", "Bo", "Lee", "Berlin"]);
});

it("lands against the filter as it stands when the answer arrives", async () => {
  const { entry, answer } = held(PROPOSAL);
  mount([entry]);
  const user = userEvent.setup();

  // Asked on the calm start, and the reader starts building while it is read:
  // the box folds above their row, still open on the wait.
  await ask(user);
  expect(
    screen.getByRole("button", { name: "Propose conditions" }),
  ).toHaveAttribute("aria-busy", "true");
  await addOwn(user, "Ann");
  expect(
    screen.getByText(en["filters.describeChanges"]).closest("details")?.open,
  ).toBe(true);
  expect(
    screen.getByText(en["filters.propose.busy"], { selector: ".pending-note" }),
  ).toBeVisible();
  const own = screen.getByLabelText<HTMLInputElement>("Value");
  answer();

  expect(await screen.findByText(barTitle(2))).toBeTruthy();
  expect(values()).toEqual(["Ann", "Lee", "Berlin"]);
  // The reader was typing in their own row, and the answer leaves them there.
  expect(document.activeElement).toBe(own);

  // Undo returns to the filter the answer landed on, not the empty one it
  // was asked from.
  await user.click(screen.getByRole("button", { name: en["common.undo"] }));
  expect(values()).toEqual(["Ann"]);
});

it("makes a proposed row the reader's own once they change it, and keeps the rest marked", async () => {
  mount([
    {
      body: {
        ...PROPOSAL,
        filter: {
          and: [
            ...PROPOSAL.filter.and,
            { field: "city", op: "neq", value: "Hamburg" },
          ],
        },
      },
    },
  ]);
  const user = userEvent.setup();
  await ask(user);
  expect(await screen.findByText(barTitle(3))).toBeTruthy();

  await user.type(screen.getAllByLabelText("Value")[0], " Jr");
  expect(screen.getByText(barTitle(2))).toBeTruthy();
  expect(
    screen.getAllByLabelText("Value")[0].closest(".filter-clause"),
  ).not.toHaveAttribute("data-proposed");

  // Removing one takes only that row; the other is still a proposal.
  const removes = screen.getAllByRole("button", { name: /^Remove / });
  await user.click(removes[1]);
  expect(values()).toEqual(["Lee Jr", "Hamburg"]);
  expect(proposedRows()).toHaveLength(1);
  expect(screen.getByText(barTitle(1))).toBeTruthy();

  await user.type(screen.getAllByLabelText("Value")[1], "er");
  expect(proposedRows()).toHaveLength(0);
  expect(screen.queryByRole("button", { name: "Keep all" })).toBeNull();
});

it("replaces only untouched rows on a second proposal, and undoes to before the first", async () => {
  const second = "contacts named Kim";
  mount([
    { body: PROPOSAL },
    {
      body: {
        ...PROPOSAL,
        filter: { and: [{ field: "full_name", op: "eq", value: "Kim" }] },
      },
    },
  ]);
  const user = userEvent.setup();
  await ask(user);
  expect(await screen.findByText(barTitle(2))).toBeTruthy();
  await user.type(screen.getAllByLabelText("Value")[1], " Mitte");

  await ask(user, second);

  expect(await screen.findByText(barTitle(1, second))).toBeTruthy();
  // The row the reader edited is theirs and stays; the untouched one went.
  expect(values()).toEqual(["Berlin Mitte", "Kim"]);

  await user.click(screen.getByRole("button", { name: en["common.undo"] }));
  expect(values()).toEqual([]);
});

it("saves proposed rows as plain conditions, and says so before the press", async () => {
  const { written } = mount();
  const user = userEvent.setup();
  await ask(user);

  expect(
    await screen.findByText(
      "2 proposed conditions in this filter. Saving keeps them.",
    ),
  ).toBeTruthy();
  const save = screen.getByRole("button", { name: en["filters.save"] });
  expect(save).toBeEnabled();
  await user.click(save);
  await user.type(
    await screen.findByRole("textbox", { name: "Name" }),
    "Lees in Berlin",
  );
  await user.click(screen.getByRole("button", { name: "Save view" }));

  await waitFor(() => expect(sentTo(written, "/views")).toHaveLength(1));
  const [posted] = sentTo(written, "/views");
  expect(posted).toMatchObject({ query: { filter: PROPOSAL.filter } });
  expect(JSON.stringify(posted)).not.toContain("proposed");
});

it("saves the filter the reader saw, not an answer that lands behind the dialog", async () => {
  const { entry, answer } = held(PROPOSAL);
  const { written } = mount([entry]);
  const user = userEvent.setup();
  await addOwn(user, "Ann");
  await ask(user);
  await user.click(screen.getByRole("button", { name: en["filters.save"] }));
  const name = await screen.findByRole("textbox", { name: "Name" });
  await user.type(name, "Anns");

  answer();
  expect(await screen.findByText(barTitle(2))).toBeTruthy();
  // The reader is still naming their save; the answer did not pull them out.
  expect(document.activeElement).toBe(name);
  await user.click(screen.getByRole("button", { name: "Save view" }));

  await waitFor(() => expect(sentTo(written, "/views")).toHaveLength(1));
  const [posted] = sentTo(written, "/views") as [
    { query: { filter: unknown } },
  ];
  expect(posted.query.filter).toEqual({
    and: [{ field: "full_name", op: "eq", value: "Ann" }],
  });
});

it("opens the folded box when an answer cannot be used, so the reason is not shut inside it", async () => {
  const refused = held({});
  const unreadable = held({ ...PROPOSAL, filter: { nonsense: 1 } });
  mount([{ ...refused.entry, status: 500 }, unreadable.entry]);
  const user = userEvent.setup();
  await addOwn(user, "Ann");
  const summary = screen.getByText(en["filters.describeChanges"]);
  const box = summary.closest("details");

  // Each time, the reader folds the box while the model reads.
  await ask(user);
  await user.click(summary);
  expect(box?.open).toBe(false);
  refused.answer();
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(box?.open).toBe(true);

  await ask(user, "contacts nobody can read");
  await user.click(summary);
  unreadable.answer();
  expect(
    await screen.findByText(en["filters.propose.unreadable"]),
  ).toBeVisible();
  expect(box?.open).toBe(true);
  expect(values()).toEqual(["Ann"]);
});

it("names what an answer could not use, also on the calm start, until the next ask", async () => {
  const later = held(NOTHING_USABLE);
  mount([{ body: NOTHING_USABLE }, later.entry]);
  const user = userEvent.setup();

  await ask(user);

  expect(
    await screen.findByText(en["filters.propose.unusedTitle"]),
  ).toBeTruthy();
  expect(
    screen.getByText("“likely to buy”: No field records a prediction."),
  ).toBeTruthy();
  expect(
    screen.getByText(
      "“in the gold tier”: No field you can filter on here records this.",
    ),
  ).toBeTruthy();
  // Nothing was proposed, so the page stays on its calm start.
  expect(values()).toEqual([]);

  // A new ask spends the last answer's phrases before its own arrives.
  await ask(user, "contacts likely to buy");
  expect(screen.queryByText(en["filters.propose.unusedTitle"])).toBeNull();
  later.answer();

  expect(
    await screen.findByText(en["filters.propose.unusedTitle"]),
  ).toBeTruthy();
  await user.click(
    screen.getByRole("button", { name: en["filters.propose.unusedDismiss"] }),
  );
  expect(screen.queryByText(en["filters.propose.unusedTitle"])).toBeNull();
});

it("gives way to a line saying a model is needed, and building by hand still works", async () => {
  mount([{ status: 409, body: { code: "ai_not_configured" } }]);
  const user = userEvent.setup();

  await ask(user);

  expect(await screen.findByText(en["filters.propose.noModel"])).toBeTruthy();
  expect(screen.queryByLabelText("Describe the contacts you want")).toBeNull();
  await user.click(
    screen.getByRole("button", { name: en["filters.addClause"] }),
  );
  expect(values()).toEqual([""]);
});
