// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Mock, vi } from "vitest";
import {
  type BackstopOptions,
  backstopAnswer,
  withSession,
} from "./company.fixtures";

/**
 * Stubs global `fetch`: the shell's reads answer from `options` or a default,
 * `responder` the rest. A refusal test passes `rollup` as a whole `Response`.
 */
export function stubFetch(
  responder: (
    url: string,
    method: string,
    request: Request,
  ) => Promise<Response>,
  options?: BackstopOptions,
): {
  fetchMock: Mock<(request: Request) => Promise<Response>>;
  urls: string[];
} {
  const urls: string[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    urls.push(request.url);
    const pathname = new URL(request.url).pathname;
    const answer =
      backstopAnswer(pathname, options) ??
      (await responder(request.url, request.method, request));
    return pathname.endsWith("/me") ? withSession(answer) : answer;
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, urls };
}
