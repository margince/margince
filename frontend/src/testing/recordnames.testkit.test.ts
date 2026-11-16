// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { recordNamesRoutes } from "./recordnames.testkit";

it("answers only requested ids of the requested kind", async () => {
  const routes = recordNamesRoutes({
    contact: { one: "Dana", two: "Anna" },
    company: { one: "Acme" },
  });
  const response = routes["GET /records/references"];
  expect(
    await (
      await response(
        null,
        new URL(
          "https://story.test/v1/records/references?kind=contact&id=one&id=missing",
        ),
      )
    ).json(),
  ).toEqual({ data: [{ id: "one", name: "Dana", archived: false }] });
  expect(
    await (
      await response(
        null,
        new URL("https://story.test/v1/records/references?kind=deal&id=one"),
      )
    ).json(),
  ).toEqual({ data: [] });
});
