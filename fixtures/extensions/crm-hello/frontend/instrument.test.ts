// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "vitest";
import { instrument } from "./screen";

test("the smoke fixture's browser instrument starts and returns hooks without throwing", () => {
  const hooks = instrument({
    version: "",
    edge: "/v1/public/instrument/crm-hello/",
  });
  expect(hooks).toBeTypeOf("object");
});
