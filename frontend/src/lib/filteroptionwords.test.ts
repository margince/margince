// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { optionWords } from "./filterdata";

const vi = (key: MessageKey) => translate("vi", key);

it("words a closed value as the record page does, and leaves an unknown one as it is", () => {
  expect(optionWords("status", "engaged", vi)).toBe(
    translate("vi", "lead.status.engaged"),
  );
  expect(optionWords("status", "engaged", vi)).not.toBe("engaged");
  expect(optionWords("status", "no_such_value", vi)).toBe("no_such_value");
});
