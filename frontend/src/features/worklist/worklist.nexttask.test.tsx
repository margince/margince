/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { readingsDay, taskRow } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { NextTaskButton } from "./worklist.nexttask";
import type { Worklist, WorklistItem } from "./worklist.queries";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function onContact(id: string): WorklistItem {
  return {
    ...taskRow(`t-${id}`, `Call ${id}`),
    subject: { type: "contact", id, label: id },
  };
}

function page(rows: WorklistItem[], next_cursor?: string): Worklist {
  return { ...readingsDay({}, rows), next_cursor };
}

/** Serves each read of the list the next answer, and records what it asked. */
function serveList(...answers: Worklist[]): URLSearchParams[] {
  const asked: URLSearchParams[] = [];
  stubApi({
    "GET /worklist": (_body, query) => {
      asked.push(query);
      const answer = answers[Math.min(asked.length, answers.length) - 1];
      return jsonResponse(answer);
    },
  });
  return asked;
}

const nextTask = () =>
  screen.findByRole("button", { name: en["brief.queue.next"] });

describe("Next task on a record", () => {
  it("opens the next record in the list, then the one after it once that is finished", async () => {
    window.location.hash = "#/contacts/a";
    // The second read is fresh: A and B are finished, so the cached A is not next.
    serveList(
      page([onContact("a"), onContact("b"), onContact("c")]),
      page([onContact("a"), onContact("b"), onContact("c")]),
      page([onContact("c")]),
    );
    render(<NextTaskButton />);
    const user = userEvent.setup();

    await user.click(await nextTask());
    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );

    await user.click(await nextTask());
    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/c?from=worklist"),
    );
  });

  it("continues the drawer's filter, scope and owner the record carries", async () => {
    window.location.hash =
      "#/contacts/a?filter=tasks&from=worklist&queue_scope=team";
    const asked = serveList(page([onContact("a"), onContact("b")]));
    render(<NextTaskButton />);

    await userEvent.setup().click(await nextTask());

    await waitFor(() =>
      expect(window.location.hash).toBe(
        "#/contacts/b?filter=tasks&from=worklist&queue_scope=team",
      ),
    );
    expect(asked.length).toBeGreaterThan(0);
    for (const query of asked) {
      expect(query.get("filter")).toBe("tasks");
      expect(query.get("scope")).toBe("team");
    }
  });

  it("reads a named owner's queue by the owner alone", async () => {
    window.location.hash = "#/contacts/a?from=worklist&owner=u-7";
    const asked = serveList(page([onContact("b")]));
    render(<NextTaskButton />);

    await userEvent.setup().click(await nextTask());

    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist&owner=u-7"),
    );
    expect(asked[0]?.get("owner")).toBe("u-7");
    expect(asked[0]?.has("scope")).toBe(false);
  });

  it("continues the reader's own whole list on a record not opened from one", async () => {
    window.location.hash = "#/contacts/a?filter=meetings";
    const asked = serveList(page([onContact("b")]));
    render(<NextTaskButton />);

    await userEvent.setup().click(await nextTask());

    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );
    expect(asked[0]?.get("scope")).toBe("mine");
    expect(asked[0]?.get("filter")).toBe("all");
  });

  it("reads on past a page with no row on another record", async () => {
    window.location.hash = "#/contacts/a";
    const asked = serveList(
      page([onContact("a")], "p2"),
      page([onContact("a")], "p2"),
      page([onContact("b")]),
    );
    render(<NextTaskButton />);

    await userEvent.setup().click(await nextTask());

    await waitFor(() =>
      expect(window.location.hash).toBe("#/contacts/b?from=worklist"),
    );
    expect(asked.at(-1)?.get("cursor")).toBe("p2");
  });

  it("reads Worklist done, and refuses the press, with no row on another record", async () => {
    window.location.hash = "#/contacts/a";
    const privacyCase: WorklistItem = {
      ...taskRow("case-1", "Answer the request"),
      source: "dsr",
    };
    serveList(page([onContact("a"), privacyCase]));
    render(<NextTaskButton />);

    const done = await screen.findByRole("button", {
      name: en["brief.queue.nextDone"],
    });
    expect(done).toHaveProperty("disabled", true);
  });

  it("opens nothing when the fresh read fails, and says so", async () => {
    window.location.hash = "#/contacts/a";
    let reads = 0;
    stubApi({
      "GET /worklist": () => {
        reads += 1;
        return reads === 1
          ? jsonResponse(page([onContact("a"), onContact("b")]))
          : jsonResponse({ title: "Unavailable", status: 503 }, 503);
      },
    });
    render(<NextTaskButton />);

    await userEvent.setup().click(await nextTask());

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(window.location.hash).toBe("#/contacts/a");
  });
});
