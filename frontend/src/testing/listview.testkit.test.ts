// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { openView } from "./listview.testkit";

it("refuses the userEvent module in place of the test's own setup()", async () => {
  await expect(openView(userEvent)).rejects.toThrow("userEvent.setup()");
});
