/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { addProposal } from "./filterpropose";
import { FiltersScreen } from "./filters";
import { decode, type Node, newGroup, newLeaf } from "./segmentpredicate";

// The proposal is a suggestion the builder shows: these tests are about where it
// lands, what it may not overwrite without asking, and that every phrase it could
// not use is named back rather than lost.

const VOCAB = {
  resource: "contact",
  fields: [
    {
      name: "full_name",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
  ],
};

const PROPOSAL = {
  resource: "contact",
  filter: { and: [{ field: "full_name", op: "contains", value: "Lee" }] },
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

type Answer = Readonly<{ status: number; body: unknown }>;

function mount(
  proposal: Answer = { status: 200, body: PROPOSAL },
  answered: Promise<void> = Promise.resolve(),
) {
  const previews: unknown[] = [];
  const asked: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const body = async () =>
        request ? await request.json() : JSON.parse(String(init?.body));
      const json = (payload: unknown, status = 200) =>
        new Response(JSON.stringify(payload), {
          status,
          headers: {
            "Content-Type":
              status === 200 ? "application/json" : "application/problem+json",
          },
        });
      if (url.endsWith("/v1/me")) {
        return json(meFixture({}));
      }
      if (url.includes("/filters/vocabulary")) {
        return json(VOCAB);
      }
      if (url.includes("/filters/propose")) {
        asked.push(await body());
        await answered;
        return json(proposal.body, proposal.status);
      }
      if (url.includes("/filters/preview")) {
        previews.push(await body());
        return json({
          resource: "contact",
          match_count: 4,
          columns: ["id"],
          rows: [],
          truncated: false,
        });
      }
      if (url.includes("/views")) {
        return json({ data: [], page: { next_cursor: null, has_more: false } });
      }
      return json({});
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider>{children}</LocaleProvider>
    </QueryClientProvider>
  );
  return { previews, asked, wrapper };
}

afterEach(cleanup);

async function describeList(user: ReturnType<typeof userEvent.setup>) {
  await user.type(
    await screen.findByLabelText("Describe the list in plain words"),
    "contacts named Lee who are likely to buy",
  );
  await user.click(screen.getByRole("button", { name: "Propose filter" }));
}

it("loads a proposal into an empty builder and names what it could not use", async () => {
  const { previews, asked, wrapper } = mount();
  const user = userEvent.setup();
  render(<FiltersScreen />, { wrapper });

  await describeList(user);

  const value = await screen.findByLabelText("Value");
  expect((value as HTMLInputElement).value).toBe("Lee");
  expect(asked).toEqual([
    {
      resource: "contact",
      text: "contacts named Lee who are likely to buy",
      locale: "en",
    },
  ]);
  // The count follows the proposed tree the way it follows any edit.
  expect(await screen.findByText("4 contacts match")).toBeTruthy();
  expect(previews.at(-1)).toMatchObject({ filter: PROPOSAL.filter });
  // The model's own reason, and the server's drop explained from its code.
  expect(
    screen.getByText("“likely to buy”: No field records a prediction."),
  ).toBeTruthy();
  expect(
    screen.getByText(
      "“in the gold tier”: No field you can filter on here records this.",
    ),
  ).toBeTruthy();
});

it("asks before a proposal touches a filter the reader already built", async () => {
  const { wrapper } = mount();
  const user = userEvent.setup();
  render(<FiltersScreen />, { wrapper });

  await user.click(await screen.findByRole("button", { name: "Add clause" }));
  await user.type(screen.getByLabelText("Value"), "Ann");
  await describeList(user);

  expect(await screen.findByText("A filter is ready")).toBeTruthy();
  // Nothing replaced yet: the reader's own clause is still the only one.
  expect(
    screen.getAllByLabelText("Value").map((v) => (v as HTMLInputElement).value),
  ).toEqual(["Ann"]);

  await user.click(
    screen.getByRole("button", { name: "Add to current filter" }),
  );
  expect(
    screen.getAllByLabelText("Value").map((v) => (v as HTMLInputElement).value),
  ).toEqual(["Ann", "Lee"]);
});

it("asks when the reader built a filter while the proposal was being read", async () => {
  let answer = () => {};
  const { wrapper } = mount(
    undefined,
    new Promise<void>((resolve) => {
      answer = resolve;
    }),
  );
  const user = userEvent.setup();
  render(<FiltersScreen />, { wrapper });

  // Asked on an empty builder, and the reader keeps working while it is read.
  await describeList(user);
  await user.click(screen.getByRole("button", { name: "Add clause" }));
  await user.type(screen.getByLabelText("Value"), "Ann");
  answer();

  expect(await screen.findByText("A filter is ready")).toBeTruthy();
  expect(
    screen.getAllByLabelText("Value").map((v) => (v as HTMLInputElement).value),
  ).toEqual(["Ann"]);
});

it("replaces the reader's filter only when they choose to", async () => {
  const { wrapper } = mount();
  const user = userEvent.setup();
  render(<FiltersScreen />, { wrapper });

  await user.click(await screen.findByRole("button", { name: "Add clause" }));
  await user.type(screen.getByLabelText("Value"), "Ann");
  await describeList(user);

  await user.click(
    await screen.findByRole("button", { name: "Replace current filter" }),
  );
  expect(
    screen.getAllByLabelText("Value").map((v) => (v as HTMLInputElement).value),
  ).toEqual(["Lee"]);
  expect(screen.queryByText("A filter is ready")).toBeNull();
});

it("says a model is needed when the installation has none", async () => {
  const { wrapper } = mount({
    status: 409,
    body: { code: "ai_not_configured", detail: "server words", status: 409 },
  });
  const user = userEvent.setup();
  render(<FiltersScreen />, { wrapper });

  await describeList(user);

  expect(
    await screen.findByText(
      "Plain-words filters need an AI model configured. You can still build the filter by hand.",
    ),
  ).toBeTruthy();
  await waitFor(() => expect(screen.queryByLabelText("Value")).toBeNull());
});

function encodedOf(node: Node): unknown {
  return JSON.parse(
    JSON.stringify(node, (key, v) => (key === "id" ? undefined : v)),
  );
}

it("adds a proposal joined the same way into the root, and any other as one group", () => {
  const current = newGroup("and", [newLeaf("full_name", "eq", "Ann")]);
  const sameJoin = decode({
    and: [{ field: "city", op: "eq", value: "Berlin" }],
  });
  const otherJoin = decode({
    or: [
      { field: "city", op: "eq", value: "Berlin" },
      { field: "city", op: "eq", value: "Wien" },
    ],
  });
  if (sameJoin === null || otherJoin === null) {
    throw new Error("fixture trees did not decode");
  }
  expect(encodedOf(addProposal(current, sameJoin))).toMatchObject({
    join: "and",
    children: [{ field: "full_name" }, { field: "city" }],
  });
  const grouped = encodedOf(addProposal(current, otherJoin));
  expect(grouped).toMatchObject({
    join: "and",
    children: [{ field: "full_name" }, { join: "or" }],
  });
});
