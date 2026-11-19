// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { translate } from "../../i18n";
import { domainQuestionReason } from "./worklist.domaincopy";

it("says a foreign site names a different business, in every language", () => {
  for (const language of ["en", "de", "vi"] as const) {
    const said = domainQuestionReason(
      (key, values) => translate(language, key, values),
      "foreign_site",
      undefined,
    );
    expect(said).toBe(
      translate(language, "worklist.domainQuestion.reason.foreignSite"),
    );
    expect(said).not.toBe("worklist.domainQuestion.reason.foreignSite");
  }
});
