// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The answer to the batched company-name read (`GET /v1/companies?id=…`), or
 * null for any other request, so a stub can try it first and route the rest.
 *
 * It names each asked id that `names` holds and leaves the others out, which is
 * how the list hides a company the reader may not see.
 */
export function companyNamesAnswer(
  url: string,
  names: Readonly<Record<string, string>>,
): Response | null {
  const asked = new URL(url, "https://test.local");
  const ids = asked.searchParams.getAll("id");
  if (asked.pathname !== "/v1/companies" || ids.length === 0) {
    return null;
  }
  return Response.json({
    data: ids.flatMap((id) => {
      const name = names[id];
      return name === undefined ? [] : [{ id, display_name: name }];
    }),
    next_cursor: null,
  });
}
