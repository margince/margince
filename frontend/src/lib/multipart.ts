// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { throwProblem } from "./problem";

// postMultipart sends a file upload and answers the parsed body, or throws the
// server's problem. The generated client serializes JSON only, so a file part
// is the one case that reaches the API through fetch by hand.
//
// `headers` carries what the endpoint declares beside the file, such as the
// If-Match precondition of a conditional upload.
//
// The body is `unknown` until a caller reads it. A 2xx whose body does not
// parse answers undefined: the server has stored the bytes, so it is not a
// failure.
export async function postMultipart(
  path: string,
  parts: Readonly<Record<string, string | Blob>>,
  headers?: Readonly<Record<string, string>>,
): Promise<unknown> {
  const body = new FormData();
  for (const [name, value] of Object.entries(parts)) {
    body.append(name, value);
  }
  // contract-fetch:allow multipart: the generated client cannot carry a file part
  const response = await fetch(path, {
    method: "POST",
    body,
    headers,
    credentials: "include",
  });
  const payload: unknown = await response.json().catch(() => undefined);
  if (!response.ok) {
    throwProblem(payload);
  }
  return payload;
}
