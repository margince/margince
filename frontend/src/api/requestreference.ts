// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { isRecord } from "../lib/problembody";

const REQUEST_ID_HEADER = "X-Request-Id";

// Copies the X-Request-Id of a failed answer into its problem body as
// `request_id`. Readers of a problem see the body and never the headers.
// problemMessageOf then shows it as "Reference: <id>".
//
// Any other answer is returned as it came, without waiting a tick, so no
// caller sees its timing change.
export function withRequestReference(
  response: Response,
): Response | Promise<Response> {
  const id = response.headers.get(REQUEST_ID_HEADER);
  const contentType = response.headers.get("Content-Type") ?? "";
  if (response.ok || !id || !contentType.includes("application/problem+json")) {
    return response;
  }
  return withReferenceInBody(response, id);
}

async function withReferenceInBody(
  response: Response,
  id: string,
): Promise<Response> {
  let problem: unknown;
  try {
    problem = await response.clone().json();
  } catch {
    return response;
  }
  if (!isRecord(problem)) return response;
  return new Response(JSON.stringify({ ...problem, request_id: id }), {
    status: response.status,
    statusText: response.statusText,
    headers: response.headers,
  });
}
