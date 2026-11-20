// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type InfiniteData,
  InfiniteQueryObserver,
  QueryClient,
} from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { hideHomeRows } from "./homerows";
import { resetToSignedOut } from "./session";
import { worklistKey } from "./workkeys";
import { homeDayOptions } from "./worklistday";

type Worklist = components["schemas"]["Worklist"];
type Day = InfiniteData<Worklist>;

const row = (id: string, source = "task") =>
  ({ id, source }) as Worklist["queue"][number];

function day(...rows: Worklist["queue"]): Day {
  return {
    pages: [{ queue: rows, focus: { items: rows } } as Worklist],
    pageParams: [undefined],
  };
}

const homeKey = homeDayOptions().queryKey;
const ids = (client: QueryClient, key: readonly unknown[] = homeKey) =>
  client
    .getQueryData<Day>(key)
    ?.pages[0]?.queue.map((r) => `${r.source}:${r.id}`);

// A Home day read that answers with whatever `server.rows` holds.
function watchHome(client: QueryClient, server: { rows: Worklist["queue"] }) {
  const observer = new InfiniteQueryObserver(client, {
    queryKey: homeKey,
    queryFn: async () => day(...server.rows).pages[0] as Worklist,
    initialPageParam: undefined as string | undefined,
    getNextPageParam: () => undefined,
  });
  return observer.subscribe(() => {});
}

describe("a row the reader acted on", () => {
  it("leaves the Home day at once, queue and Focus, and the day shows Updating", async () => {
    const client = new QueryClient();
    client.setQueryData(homeKey, day(row("a"), row("b")));
    await hideHomeRows(client, [{ source: "task", id: "a" }]);
    const page = client.getQueryData<Day>(homeKey)?.pages[0];
    expect(page?.queue.map((r) => r.id)).toEqual(["b"]);
    expect(page?.focus?.items.map((r) => r.id)).toEqual(["b"]);
    expect(page?.served_stale).toBe(true);
  });

  it("is matched by source and id, so another lane's row with that id stays", async () => {
    const client = new QueryClient();
    client.setQueryData(homeKey, day(row("x", "task"), row("x", "meeting")));
    await hideHomeRows(client, [{ source: "task", id: "x" }]);
    expect(ids(client)).toEqual(["meeting:x"]);
  });

  it("leaves every other Worklist read alone", async () => {
    const client = new QueryClient();
    const screen = [...worklistKey, "mine", "all", "", "today"];
    const team = [...worklistKey, "team", "all", "", "today"];
    client.setQueryData(homeKey, day(row("a")));
    client.setQueryData(screen, day(row("a")));
    client.setQueryData(team, day(row("a")));
    await hideHomeRows(client, [{ source: "task", id: "a" }]);
    expect(ids(client, screen)).toEqual(["task:a"]);
    expect(ids(client, team)).toEqual(["task:a"]);
  });

  it("comes back from the server when the write failed, with no copy kept", async () => {
    const client = new QueryClient();
    const server = { rows: [row("a"), row("b")] };
    client.setQueryData(homeKey, day(...server.rows));
    const stop = watchHome(client, server);
    const settle = await hideHomeRows(client, [{ source: "task", id: "a" }]);
    expect(ids(client)).toEqual(["task:b"]);
    settle();
    await new Promise((r) => setTimeout(r, 0));
    expect(ids(client)).toEqual(["task:a", "task:b"]);
    expect(
      client.getQueryData<Day>(homeKey)?.pages[0]?.served_stale,
    ).toBeUndefined();
    stop();
  });

  it("ends with the server's truth when two actions run and one fails", async () => {
    const client = new QueryClient();
    const server = { rows: [row("a"), row("b"), row("c")] };
    client.setQueryData(homeKey, day(...server.rows));
    const stop = watchHome(client, server);
    const first = await hideHomeRows(client, [{ source: "task", id: "a" }]);
    const second = await hideHomeRows(client, [{ source: "task", id: "b" }]);
    expect(ids(client)).toEqual(["task:c"]);
    server.rows = [row("a"), row("c")];
    second();
    first();
    await new Promise((r) => setTimeout(r, 0));
    expect(ids(client)).toEqual(["task:a", "task:c"]);
    stop();
  });

  it("writes nothing for a member who signed out while it waited", async () => {
    const client = new QueryClient();
    client.setQueryData(homeKey, day(row("a"), row("b")));
    const hiding = hideHomeRows(client, [{ source: "task", id: "a" }]);
    await resetToSignedOut(client);
    const settle = await hiding;
    settle();
    expect(client.getQueryData(homeKey)).toBeUndefined();
  });
});
