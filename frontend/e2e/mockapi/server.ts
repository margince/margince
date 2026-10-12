// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Route } from "@playwright/test";
import type { MockState } from "./state";

export type Reply = (body: unknown, status?: number) => Promise<void>;

// One intercepted call to /v1, as every handler sees it.
export type MockRequest = Readonly<{
  route: Route;
  url: URL;
  // The path below /v1, the form every route row is written in.
  path: string;
  method: string;
  // The capture groups of a RegExp row, in order.
  params: readonly string[];
  json: Reply;
}>;

// What a handler resolves to when it hands the call on to the next row. Any
// other value means the handler fulfilled the route.
export const PASS = Symbol("pass");

export type Handler = (
  request: MockRequest,
  state: MockState,
) => Promise<unknown>;

// A string is an exact path, a RegExp captures `params`, and a function
// covers the prefix and suffix rows.
export type PathPattern = string | RegExp | ((path: string) => boolean);

export type MockRoute = Readonly<{
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  path: PathPattern;
  // A condition on the page's options rather than on the request.
  when?: (state: MockState) => boolean;
  handle: Handler;
}>;

export function page(data: unknown[]) {
  return { data, page: { next_cursor: null } };
}

// A handler for a fixed answer. JSON.stringify runs on every call, so a
// body that changes between calls is still read fresh.
export function reply(body: unknown, status = 200): Handler {
  return ({ json }) => json(body, status);
}

export const notFound = {
  type: "about:blank",
  title: "Not Found",
  status: 404,
};

function matchPath(pattern: PathPattern, path: string): string[] | null {
  if (typeof pattern === "string") {
    return pattern === path ? [] : null;
  }
  if (pattern instanceof RegExp) {
    return pattern.exec(path)?.slice(1) ?? null;
  }
  return pattern(path) ? [] : null;
}

function matchRow(
  row: MockRoute,
  request: Omit<MockRequest, "params">,
  state: MockState,
): string[] | null {
  if (row.method !== undefined && row.method !== request.method) {
    return null;
  }
  if (row.when !== undefined && !row.when(state)) {
    return null;
  }
  return matchPath(row.path, request.path);
}

// The rows are tried in order, and the first one that answers wins. A broad
// prefix row sits below the exact rows it would swallow. A call no row
// answers gets an empty list page.
export async function answer(
  routes: readonly MockRoute[],
  route: Route,
  state: MockState,
): Promise<void> {
  const url = new URL(route.request().url());
  const json: Reply = (body, status = 200) =>
    route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  const request = {
    route,
    url,
    path: url.pathname.replace(/^\/v1/, ""),
    method: route.request().method(),
    json,
  };
  for (const row of routes) {
    const params = matchRow(row, request, state);
    if (
      params !== null &&
      (await row.handle({ ...request, params }, state)) !== PASS
    ) {
      return;
    }
  }
  await json(page([]));
}
