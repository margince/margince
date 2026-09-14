/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { LinkedCaseNotice } from "./privacy.caselink.notice";

// A link that names a case the page cannot show is the failure this notice
// exists for: without it the officer reads an ordinary queue and nothing says
// their link named anything.

afterEach(cleanup);

function draw(linked: Parameters<typeof LinkedCaseNotice>[0]["linked"]) {
  render(
    <LocaleProvider>
      <LinkedCaseNotice linked={linked} />
    </LocaleProvider>,
  );
}

describe("the notice for a case the page cannot show", () => {
  it("says so when the case is on no page that loaded", () => {
    draw({ kind: "absent", id: "case-9" });
    expect(screen.getByText(en["privacy.caseNotHere"])).toBeInTheDocument();
  });

  // Not absent, merely not here yet: the reader can press Load more, so a
  // notice claiming the case is missing would be wrong and they could disprove
  // it by pressing one button.
  it("stays quiet while more pages remain to load", () => {
    draw({ kind: "loading", id: "case-9" });
    expect(screen.queryByText(en["privacy.caseNotHere"])).toBeNull();
  });

  it("stays quiet when the case is on screen", () => {
    draw({ kind: "shown", id: "case-9" });
    expect(screen.queryByText(en["privacy.caseNotHere"])).toBeNull();
  });

  it("stays quiet when the address names no case at all", () => {
    draw({ kind: "none" });
    expect(screen.queryByText(en["privacy.caseNotHere"])).toBeNull();
  });
});
