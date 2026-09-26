/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { AnalyticsScreen } from "./analytics";
import type { AnalyticsScope } from "./analytics.context";
import { QuestionsView } from "./analytics.questions";
import {
  ANSWER,
  CONTEXT,
  QUERY,
  SCHEMA,
  STAGES,
  USERS,
} from "./analytics.questions.testkit";
import { render, reportsStub } from "./analytics.testkit";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Asked = { method: string; url: string; body: unknown };

function stubServer(
  answers: Readonly<Record<string, () => Response>> = {},
): Asked[] {
  const asked: Asked[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const method = request?.method ?? init?.method ?? "GET";
      const body =
        method === "GET"
          ? null
          : request
            ? await request.json()
            : JSON.parse(String(init?.body));
      asked.push({ method, url, body });
      const path = new URL(url, "http://local").pathname.replace(/^\/v1/, "");
      const answer = answers[`${method} ${path}`];
      if (answer) return answer();
      if (path === "/analytics/schema") return jsonResponse(SCHEMA);
      if (path === "/stages") return jsonResponse(STAGES);
      if (path === "/users") return jsonResponse(USERS);
      if (path === "/analytics/query") return jsonResponse(ANSWER);
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }),
  );
  return asked;
}

beforeEach(() => {
  globalThis.location.hash = "#/analytics/questions";
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderView(onSelectScope: (scope: AnalyticsScope) => void = () => {}) {
  return render(
    <QuestionsView
      context={CONTEXT}
      selection={{ scope: CONTEXT.default_scope }}
      onSelectScope={onSelectScope}
    />,
  );
}

async function chooseDeals(user: ReturnType<typeof userEvent.setup>) {
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: "Report" }),
    "All deals by stage",
  );
}

describe("the Questions section", () => {
  it("is the last tab of Analytics", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    const tabs = await screen.findByRole("group", {
      name: "Analytics sections",
    });
    const names = within(tabs)
      .getAllByRole("button")
      .map((tab) => tab.textContent);
    expect(names.at(-1)).toBe("Questions");
  });

  it("says the builder is withheld when the seat can read no population", async () => {
    stubServer({
      "GET /analytics/schema": () =>
        jsonResponse({ version: "v0", entities: null }),
    });
    renderView();
    expect(
      await screen.findByText(
        "No report data is readable with your role. An administrator can grant access.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Ask" })).toBeNull();
  });

  it("asks over the page's population and shows the answer", async () => {
    const user = userEvent.setup();
    const asked = stubServer();
    renderView();
    await chooseDeals(user);
    await user.click(screen.getByRole("button", { name: "Ask" }));
    expect(
      await screen.findByRole("columnheader", { name: "Stage" }),
    ).toBeTruthy();
    const query = asked.find((call) => call.url.includes("/analytics/query"));
    expect(query?.body).toEqual({
      entity: "deals-by-stage",
      scope_kind: "workspace",
      measures: [{ fn: "count" }],
      limit: 100,
    });
  });

  it("saves with save:true and opens the saved question's address", async () => {
    const user = userEvent.setup();
    const asked = stubServer({
      "POST /analytics/query": () =>
        jsonResponse({ ...ANSWER, run_id: "run-9" }),
    });
    renderView();
    await chooseDeals(user);
    await user.click(screen.getByRole("button", { name: "Save question" }));
    await vi.waitFor(() =>
      expect(globalThis.location.hash).toBe("#/analytics/questions/run-9"),
    );
    const saved = asked.find((call) => call.url.includes("/analytics/query"));
    expect(saved?.body).toMatchObject({ save: true });
  });
});

describe("a saved question", () => {
  const run = {
    id: "run-1",
    query: {
      ...QUERY,
      filters: [{ field: "currency", op: "eq", value: "EUR" }],
    },
    answer: ANSWER,
    asked_by: "u-1",
    stored_floor: 5,
  };

  it("reads back the question and says it is answered under the reader's access", async () => {
    globalThis.location.hash = "#/analytics/questions/run-1";
    stubServer({ "GET /analytics/runs/run-1": () => jsonResponse(run) });
    renderView();
    expect(
      await screen.findByText(
        "Answered under your own access, so figures can differ from what was seen when it was saved.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("All deals by stage")).toBeTruthy();
    expect(screen.getByText("Count, Sum (Amount)")).toBeTruthy();
    expect(screen.getByText("Currency is EUR")).toBeTruthy();
    expect(await screen.findByText("Qualified")).toBeTruthy();
  });

  it("loads into the builder on Edit, over the population it was asked in", async () => {
    const user = userEvent.setup();
    globalThis.location.hash = "#/analytics/questions/run-1";
    stubServer({ "GET /analytics/runs/run-1": () => jsonResponse(run) });
    const chosen: AnalyticsScope[] = [];
    renderView((scope) => chosen.push(scope));
    await user.click(
      await screen.findByRole("button", { name: "Edit question" }),
    );
    await act(async () => {
      globalThis.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    expect(globalThis.location.hash).toBe("#/analytics/questions");
    expect(
      (await screen.findByRole("combobox", { name: "Report" })).textContent,
    ).toBe("All deals by stage");
    expect(chosen).toEqual([CONTEXT.default_scope]);
  });

  it("says a refused read in the reader's words", async () => {
    globalThis.location.hash = "#/analytics/questions/run-1";
    stubServer({
      "GET /analytics/runs/run-1": () =>
        jsonResponse({ status: 403, code: "permission_denied" }, 403),
    });
    renderView();
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByRole("button", { name: "New question" })).toBeTruthy();
  });
});
