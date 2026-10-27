/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { navigate } from "../../lib/router";
import { readingsDay, taskRow } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { NextTaskButton } from "./worklist.nexttask";
import type { Worklist, WorklistItem } from "./worklist.queries";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function onContact(id: string, task = 1): WorklistItem {
  return {
    ...taskRow(`t-${id}-${task}`, `Call ${id}`),
    subject: { type: "contact", id, label: id },
  };
}

function noticeOn(id: string): WorklistItem {
  return { ...onContact(id, 9), source: "notice_case" };
}

const privacyCase: WorklistItem = {
  ...taskRow("case-1", "Answer the request"),
  source: "dsr",
};

function page(rows: WorklistItem[], next_cursor?: string): Worklist {
  return { ...readingsDay({}, rows), next_cursor };
}

/** Serves the list's pages by the cursor asked for, and records each read. */
function serveList(...pages: Worklist[]): URLSearchParams[] {
  const asked: URLSearchParams[] = [];
  const byCursor = new Map(
    pages.map((answer, index) => [pages[index - 1]?.next_cursor ?? "", answer]),
  );
  stubApi({
    "GET /worklist": (_body, query) => {
      asked.push(query);
      return jsonResponse(byCursor.get(query.get("cursor") ?? ""));
    },
  });
  return asked;
}

const nextTask = () =>
  screen.findByRole("button", { name: en["brief.queue.next"] });

async function pressOn(hash: string) {
  window.location.hash = hash;
  render(<NextTaskButton />);
  await userEvent.setup().click(await nextTask());
}

describe("Next task on a record", () => {
  it.each([
    ["a", "b"],
    ["b", "c"],
    ["c", "a"],
    ["gone", "a"],
  ])(
    "on contact %s with A, B and C open, opens contact %s",
    async (on, opens) => {
      serveList(page([onContact("a"), onContact("b"), onContact("c")]));
      await pressOn(`#/contacts/${on}`);
      await waitFor(() =>
        expect(window.location.hash).toBe(`#/contacts/${opens}?from=worklist`),
      );
    },
  );

  it("orders records by their first row, so a later row on the record on screen does not move it", async () => {
    serveList(
      page([onContact("a"), onContact("b"), onContact("a", 2), onContact("c")]),
    );
    await pressOn("#/contacts/a");
    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );
  });

  it.each([
    ["a", "b"],
    ["b", "c"],
    ["c", "a"],
  ])(
    "with notices trailing in reverse order, on contact %s opens contact %s",
    async (on, opens) => {
      serveList(
        page([
          onContact("a"),
          onContact("a", 2),
          onContact("b"),
          onContact("c"),
          onContact("c", 2),
          onContact("c", 3),
          noticeOn("c"),
          noticeOn("b"),
          noticeOn("a"),
        ]),
      );
      await pressOn(`#/contacts/${on}`);
      await waitFor(() =>
        expect(window.location.hash).toBe(`#/contacts/${opens}?from=worklist`),
      );
    },
  );

  it("reads on to the page holding the record on screen before choosing", async () => {
    const asked = serveList(
      page([onContact("b")], "p2"),
      page([onContact("a"), onContact("c")]),
    );
    await pressOn("#/contacts/a");
    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/c?from=worklist"),
    );
    expect(asked.map((query) => query.get("cursor"))).toEqual([null, "p2"]);
  });

  it("wraps to the first page once the pages after the record on screen end", async () => {
    serveList(page([onContact("b")], "p2"), page([onContact("a")]));
    await pressOn("#/contacts/a");
    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );
  });

  it("reads the list only when pressed", async () => {
    const asked = serveList(page([onContact("a"), onContact("b")]));
    window.location.hash = "#/contacts/a";
    render(<NextTaskButton />);
    const button = await nextTask();
    expect(asked).toHaveLength(0);

    await userEvent.setup().click(button);

    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );
    expect(asked).toHaveLength(1);
  });

  it("is busy while it reads and takes no second press", async () => {
    const asked: URLSearchParams[] = [];
    let answer: (response: Response) => void = () => undefined;
    stubApi({
      "GET /worklist": (_body, query) => {
        asked.push(query);
        return new Promise<Response>((resolve) => {
          answer = resolve;
        });
      },
    });
    window.location.hash = "#/contacts/a";
    render(<NextTaskButton />);
    const user = userEvent.setup();
    const button = await nextTask();

    await user.click(button);
    await waitFor(() => expect(button.getAttribute("aria-busy")).toBe("true"));
    await user.click(button);
    expect(asked).toHaveLength(1);

    answer(jsonResponse(page([onContact("b")])));
    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );
  });

  it("says No more records when only this record and rows off record pages are left, and a press then reads nothing", async () => {
    const asked = serveList(page([onContact("a"), privacyCase]));
    await pressOn("#/contacts/a");

    const none = await screen.findByRole("button", {
      name: en["brief.queue.nextNone"],
    });
    expect(none).toHaveProperty("disabled", true);
    await userEvent.setup().click(none);
    expect(asked).toHaveLength(1);
    expect(window.location.hash).toBe("#/contacts/a");
  });

  it("offers the press again on the next page the reader opens", async () => {
    serveList(page([onContact("a")]));
    await pressOn("#/contacts/a");
    await screen.findByRole("button", { name: en["brief.queue.nextNone"] });

    act(() => navigate({ screen: "contacts", id: "z" }));

    expect(await nextTask()).toHaveProperty("disabled", false);
  });

  it("offers the press again on a record the reader comes back to", async () => {
    const asked = serveList(page([onContact("a")]));
    await pressOn("#/contacts/a");
    await screen.findByRole("button", { name: en["brief.queue.nextNone"] });

    act(() => navigate({ screen: "contacts", id: "b" }));
    expect(await nextTask()).toHaveProperty("disabled", false);
    act(() => navigate({ screen: "contacts", id: "a" }));
    await waitFor(() => expect(window.location.hash).toBe("#/contacts/a"));

    expect(await nextTask()).toHaveProperty("disabled", false);
    expect(asked).toHaveLength(1);
  });

  it("leaves a reader who opened another record while the list was read where they are", async () => {
    let answer: (response: Response) => void = () => undefined;
    stubApi({
      "GET /worklist": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    window.location.hash = "#/contacts/a";
    render(<NextTaskButton />);
    const user = userEvent.setup();
    const button = await nextTask();
    await user.click(button);
    await waitFor(() => expect(button.getAttribute("aria-busy")).toBe("true"));

    act(() => navigate({ screen: "contacts", id: "z" }));
    await waitFor(() =>
      expect(screen.getByRole("button").getAttribute("aria-busy")).toBeNull(),
    );
    const list = jsonResponse(page([onContact("a"), onContact("b")]));
    answer(list);
    await waitFor(() => expect(list.bodyUsed).toBe(true));
    await act(async () => undefined);

    expect(window.location.hash).toBe("#/contacts/z");
  });

  it("drops a failed read's error line on the next record", async () => {
    stubApi({
      "GET /worklist": () =>
        jsonResponse({ title: "Unavailable", status: 503 }, 503),
    });
    await pressOn("#/contacts/a");
    expect(await screen.findByRole("alert")).toBeTruthy();

    act(() => navigate({ screen: "contacts", id: "b" }));

    await nextTask();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("continues the drawer's filter, scope and owner the record carries", async () => {
    const asked = serveList(page([onContact("a"), onContact("b")]));
    await pressOn("#/contacts/a?filter=tasks&from=worklist&queue_scope=team");

    await waitFor(() =>
      expect(window.location.hash).toBe(
        "#/contacts/b?filter=tasks&from=worklist&queue_scope=team",
      ),
    );
    expect(asked).toHaveLength(1);
    expect(asked[0]?.get("filter")).toBe("tasks");
    expect(asked[0]?.get("scope")).toBe("team");
  });

  it("reads a named owner's queue by the owner alone", async () => {
    const asked = serveList(page([onContact("b")]));
    await pressOn("#/contacts/a?from=worklist&owner=u-7");

    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist&owner=u-7"),
    );
    expect(asked[0]?.get("owner")).toBe("u-7");
    expect(asked[0]?.has("scope")).toBe(false);
  });

  it("continues the reader's own whole list on a record not opened from one", async () => {
    const asked = serveList(page([onContact("b")]));
    await pressOn("#/contacts/a?filter=meetings");

    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );
    expect(asked[0]?.get("scope")).toBe("mine");
    expect(asked[0]?.get("filter")).toBe("all");
  });

  it("opens nothing when the read fails, and says so", async () => {
    stubApi({
      "GET /worklist": () =>
        jsonResponse({ title: "Unavailable", status: 503 }, 503),
    });
    await pressOn("#/contacts/a");

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(window.location.hash).toBe("#/contacts/a");
  });
});
