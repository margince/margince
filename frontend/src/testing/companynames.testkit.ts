// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

export function companyNamesAnswer(
  url: string,
  names: Readonly<Record<string, string>>,
): Response | null {
  const asked = new URL(url, "https://test.local");
  const ids = asked.searchParams.getAll("id");
  if (
    asked.pathname !== "/v1/records/references" ||
    asked.searchParams.get("kind") !== "company" ||
    ids.length === 0
  ) {
    return null;
  }
  return Response.json({
    data: ids.flatMap((id) => {
      const name = names[id];
      return name === undefined ? [] : [{ id, name, archived: false }];
    }),
  });
}
