// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { render } from "../../testing/settings.testkit";
import {
  SIGNATURE_HTML_MAX_CHARS,
  SignatureFields,
} from "./settingssignatureeditor";

afterEach(cleanup);

// "界" is one code point and one UTF-16 unit, while "𝐀" is one code point and
// two units. A counter that counted units would show early for the second.
function editor(html: string) {
  render(
    <SignatureFields
      mode="html"
      shown={{ body: "", html, title: "", phone: "", format: "html" }}
      disabled={false}
      change={() => undefined}
    />,
  );
}

it("shows no counter at 80 % of the cap or below", () => {
  editor("𝐀".repeat(SIGNATURE_HTML_MAX_CHARS * 0.8));
  expect(screen.queryByText(/characters$/)).toBeNull();
  expect(screen.getByText(/Up to 10,000 characters/)).not.toBeNull();
});

it("counts code points once the input passes 80 % of the cap", () => {
  editor(`  ${"界".repeat(8_001)}\n`);
  const counter = screen.getByText("8,001 of 10,000 characters");
  expect(counter.className).not.toContain("field-error");
});

it("shows the count in the danger tone above the cap", () => {
  editor("界".repeat(SIGNATURE_HTML_MAX_CHARS + 1));
  expect(screen.getByText("10,001 of 10,000 characters").className).toContain(
    "field-error",
  );
});
