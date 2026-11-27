/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import { MissingTranslationsLine } from "./labeltranslations";

afterEach(cleanup);

function line(
  translations: Record<string, string>,
  languages: ("en" | "de" | "vi")[],
) {
  render(
    <LocaleProvider initial="en">
      <MissingTranslationsLine
        translations={translations}
        languages={languages}
      />
    </LocaleProvider>,
  );
  return screen.queryByTestId("label-translations-missing");
}

describe("a stage with no name in a language the team reads", () => {
  it("is marked for that language", () => {
    expect(line({}, ["vi"])?.textContent).toBe(
      englishCopy["translation.missing.vi"],
    );
  });

  it("is not marked once the name is given", () => {
    expect(line({ vi: "Đã gửi báo giá" }, ["vi"])).toBeNull();
  });

  it("is not marked when no member reads another language", () => {
    expect(line({}, [])).toBeNull();
  });

  it("counts a blank name as missing, in the app's language order", () => {
    expect(line({ vi: "  " }, ["de", "vi"])?.textContent).toBe(
      "No name in German · No name in Vietnamese",
    );
  });
});
