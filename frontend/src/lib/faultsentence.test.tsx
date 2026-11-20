/** @vitest-environment happy-dom */
import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import {
  faultSentence,
  fieldCheckSentence,
  moneyLimit,
  ProblemError,
  percentLimit,
  problemFieldSentencesOf,
  problemMessageOf,
  wholeNumberFault,
} from "./problem";

const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

describe("a field fault worded from the screen's label and limit", () => {
  type Words = Readonly<Record<"en" | "de" | "vi", string>>;
  const winProbability: Words = {
    en: "Win probability",
    de: "Gewinnwahrscheinlichkeit",
    vi: "Xác suất thắng",
  };
  const fault = (code: string, params?: Record<string, string>) => ({
    field: "win_probability",
    code,
    message: "`win_probability` must be an integer, not a number",
    ...(params ? { params } : null),
  });
  const cases: ReadonlyArray<
    Readonly<{
      code: string;
      params?: Record<string, string>;
      named: Words;
      bare: Words;
    }>
  > = [
    {
      code: "required",
      named: {
        en: "Win probability is required.",
        de: "Gewinnwahrscheinlichkeit ist erforderlich.",
        vi: "Cần nhập Xác suất thắng.",
      },
      bare: {
        en: "This field is required.",
        de: "Dieses Feld ist erforderlich.",
        vi: "Trường này bắt buộc.",
      },
    },
    {
      code: "not_integer",
      named: {
        en: "Enter a whole number for Win probability.",
        de: "Gib für Gewinnwahrscheinlichkeit eine ganze Zahl ein.",
        vi: "Nhập số nguyên cho Xác suất thắng.",
      },
      bare: {
        en: "Enter a whole number.",
        de: "Gib eine ganze Zahl ein.",
        vi: "Nhập số nguyên.",
      },
    },
    {
      code: "not_number",
      named: {
        en: "Enter a number for Win probability.",
        de: "Gib für Gewinnwahrscheinlichkeit eine Zahl ein.",
        vi: "Nhập số cho Xác suất thắng.",
      },
      bare: {
        en: "Enter a number.",
        de: "Gib eine Zahl ein.",
        vi: "Nhập một số.",
      },
    },
    {
      code: "email_malformed",
      named: {
        en: "Enter an email address for Win probability.",
        de: "Gib für Gewinnwahrscheinlichkeit eine E-Mail-Adresse ein.",
        vi: "Nhập địa chỉ email cho Xác suất thắng.",
      },
      bare: {
        en: "Enter an email address.",
        de: "Gib eine E-Mail-Adresse ein.",
        vi: "Nhập địa chỉ email.",
      },
    },
    {
      code: "invalid_date",
      named: {
        en: "Enter a valid date for Win probability.",
        de: "Gib für Gewinnwahrscheinlichkeit ein gültiges Datum ein.",
        vi: "Nhập ngày hợp lệ cho Xác suất thắng.",
      },
      bare: {
        en: "Enter a valid date.",
        de: "Gib ein gültiges Datum ein.",
        vi: "Nhập ngày hợp lệ.",
      },
    },
    {
      code: "datetime_malformed",
      named: {
        en: "Enter a valid date and time for Win probability.",
        de: "Gib für Gewinnwahrscheinlichkeit ein gültiges Datum mit Uhrzeit ein.",
        vi: "Nhập ngày và giờ hợp lệ cho Xác suất thắng.",
      },
      bare: {
        en: "Enter a valid date and time.",
        de: "Gib ein gültiges Datum mit Uhrzeit ein.",
        vi: "Nhập ngày và giờ hợp lệ.",
      },
    },
    {
      code: "invalid_enum",
      named: {
        en: "Choose one of the offered values for Win probability.",
        de: "Wähle für Gewinnwahrscheinlichkeit einen der angebotenen Werte.",
        vi: "Chọn một giá trị trong danh sách cho Xác suất thắng.",
      },
      bare: {
        en: "Choose one of the offered values.",
        de: "Wähle einen der angebotenen Werte.",
        vi: "Chọn một giá trị trong danh sách.",
      },
    },
    {
      code: "invalid_type",
      named: {
        en: "Check Win probability and try again.",
        de: "Prüfe Gewinnwahrscheinlichkeit und versuche es erneut.",
        vi: "Kiểm tra Xác suất thắng và thử lại.",
      },
      bare: {
        en: "Check this value and try again.",
        de: "Prüfe diesen Wert und versuche es erneut.",
        vi: "Kiểm tra giá trị này và thử lại.",
      },
    },
    {
      code: "out_of_range",
      params: { min: "0", max: "1000" },
      named: {
        en: "Win probability must be from 0 to 1,000.",
        de: "Gewinnwahrscheinlichkeit muss zwischen 0 und 1.000 liegen.",
        vi: "Xác suất thắng phải từ 0 đến 1.000.",
      },
      bare: {
        en: "Enter a value from 0 to 1,000.",
        de: "Gib einen Wert von 0 bis 1.000 ein.",
        vi: "Nhập giá trị từ 0 đến 1.000.",
      },
    },
    {
      code: "out_of_range",
      params: { min: "0" },
      named: {
        en: "Win probability must be at least 0.",
        de: "Gewinnwahrscheinlichkeit muss mindestens 0 sein.",
        vi: "Xác suất thắng phải từ 0 trở lên.",
      },
      bare: {
        en: "Enter a value of at least 0.",
        de: "Gib einen Wert von mindestens 0 ein.",
        vi: "Nhập giá trị từ 0 trở lên.",
      },
    },
    {
      code: "out_of_range",
      params: { max: "100" },
      named: {
        en: "Win probability must be at most 100.",
        de: "Gewinnwahrscheinlichkeit darf höchstens 100 sein.",
        vi: "Xác suất thắng không được vượt quá 100.",
      },
      bare: {
        en: "Enter a value of at most 100.",
        de: "Gib einen Wert von höchstens 100 ein.",
        vi: "Nhập giá trị không vượt quá 100.",
      },
    },
    {
      code: "out_of_range",
      named: {
        en: "Win probability is outside the allowed range.",
        de: "Gewinnwahrscheinlichkeit liegt außerhalb des erlaubten Bereichs.",
        vi: "Xác suất thắng nằm ngoài phạm vi cho phép.",
      },
      bare: {
        en: "This value is outside the allowed range.",
        de: "Dieser Wert liegt außerhalb des erlaubten Bereichs.",
        vi: "Giá trị này nằm ngoài phạm vi cho phép.",
      },
    },
    {
      code: "duplicate",
      named: {
        en: "Win probability lists the same value twice. Remove the repeat.",
        de: "Gewinnwahrscheinlichkeit enthält denselben Wert zweimal. Entferne die Wiederholung.",
        vi: "Xác suất thắng có một giá trị lặp lại. Xóa giá trị lặp.",
      },
      bare: {
        en: "The same value is listed twice. Remove the repeat.",
        de: "Derselbe Wert steht zweimal in der Liste. Entferne die Wiederholung.",
        vi: "Có một giá trị bị lặp lại. Xóa giá trị lặp.",
      },
    },
    {
      code: "invalid_characters",
      named: {
        en: "Enter Win probability on one line.",
        de: "Gib Gewinnwahrscheinlichkeit in einer Zeile ein.",
        vi: "Nhập Xác suất thắng trên một dòng.",
      },
      bare: {
        en: "Enter this value on one line.",
        de: "Gib den Wert in einer Zeile ein.",
        vi: "Nhập giá trị này trên một dòng.",
      },
    },
    {
      code: "future",
      named: {
        en: "Win probability cannot be in the future.",
        de: "Gewinnwahrscheinlichkeit darf nicht in der Zukunft liegen.",
        vi: "Xác suất thắng không được là ngày trong tương lai.",
      },
      bare: {
        en: "This date cannot be in the future.",
        de: "Dieses Datum darf nicht in der Zukunft liegen.",
        vi: "Không được chọn ngày trong tương lai.",
      },
    },
    {
      code: "too_many_decimals",
      params: { max: "2" },
      named: {
        en: "Decimal places allowed in Win probability: 2.",
        de: "Erlaubte Nachkommastellen für Gewinnwahrscheinlichkeit: 2.",
        vi: "Số chữ số thập phân tối đa cho Xác suất thắng: 2.",
      },
      bare: {
        en: "Decimal places allowed: 2.",
        de: "Erlaubte Nachkommastellen: 2.",
        vi: "Số chữ số thập phân tối đa: 2.",
      },
    },
  ];

  for (const { code, params, named, bare } of cases) {
    for (const locale of ["en", "de", "vi"] as const) {
      const shown = `${code}${params ? ` ${JSON.stringify(params)}` : ""} in ${locale}`;
      it(`words ${shown} by the field's label`, () => {
        expect(
          faultSentence(fault(code, params), locale, {
            win_probability: winProbability[locale],
          }),
        ).toBe(named[locale]);
      });
      it(`words ${shown} for a form's own check with no label`, () => {
        expect(fieldCheckSentence(code, locale, undefined, params)).toBe(
          bare[locale],
        );
      });
      if (locale !== "en") {
        it(`words ${shown} without a wire key for an unlabelled field`, () => {
          expect(faultSentence(fault(code, params), locale)).toBe(bare[locale]);
        });
      }
    }
  }

  it("keeps the server's sentence for an English reader of an unlabelled field", () => {
    expect(
      faultSentence(
        {
          ...fault("not_integer"),
          message: "The probability is a whole number",
        },
        "en",
      ),
    ).toBe("The probability is a whole number.");
  });

  it("words an unlabelled field in the catalog when the server's sentence opens on its key", () => {
    expect(faultSentence(fault("not_integer"), "en")).toBe(
      "Enter a whole number.",
    );
    expect(
      faultSentence(
        { ...fault("not_integer"), message: "win_probability must be whole" },
        "en",
      ),
    ).toBe("Enter a whole number.");
  });

  it("gives an English reader the words, not the API sentence", () => {
    const english = Object.assign(t, { locale: "en" as const });
    const line = problemMessageOf(
      new ProblemError({
        status: 422,
        code: "validation_error",
        detail: "`win_probability` must be an integer, not a number",
        details: { errors: [fault("not_integer")] },
      }),
      english,
      undefined,
      { win_probability: "Win probability" },
    );
    expect(line).toBe("Enter a whole number for Win probability.");
  });

  it("states a minor-unit bound as the amount the reader typed", () => {
    const price = {
      field: "unit_price_minor",
      code: "out_of_range",
      message: "`unit_price_minor` must be a number of at least 0, not -500",
      params: { min: "0" },
    };
    expect(
      faultSentence(price, "en", {
        unit_price_minor: {
          label: "Unit price",
          limit: moneyLimit("EUR", "en"),
        },
      }),
    ).toBe("Unit price must be at least €0.00.");
  });

  it("states a percent bound with the reader's percent sign", () => {
    const discount = {
      field: "discount_pct",
      code: "out_of_range",
      message: "x",
      params: { min: "0", max: "100" },
    };
    const labels = (locale: "en" | "de") => ({
      discount_pct: { label: "Rabatt", limit: percentLimit(locale) },
    });
    expect(faultSentence(discount, "en", labels("en"))).toBe(
      "Rabatt must be from 0% to 100%.",
    );
    expect(faultSentence(discount, "de", labels("de"))).toBe(
      "Rabatt muss zwischen 0 % und 100 % liegen.",
    );
  });

  it("states a date-time span as days in the reader's form", () => {
    const line = faultSentence(
      {
        field: "valid_until",
        code: "out_of_range",
        message: "x",
        params: { min: "0001-01-02", max: "9999-12-30" },
      },
      "en",
      { valid_until: "Valid until" },
    );
    expect(line).toBe("Valid until must be from 02/01/0001 to 30/12/9999.");
  });

  it("words a form's own check as the server's refusal of that value", () => {
    const server = faultSentence(
      fault("out_of_range", { min: "0", max: "100" }),
      "de",
      {
        win_probability: winProbability.de,
      },
    );
    expect(
      fieldCheckSentence("out_of_range", "de", winProbability.de, {
        min: "0",
        max: "100",
      }),
    ).toBe(server);
  });

  it("names each field under its own label", () => {
    const error = new ProblemError({
      status: 422,
      code: "validation_error",
      detail: "x",
      details: { errors: [fault("not_integer")] },
    });
    expect(
      problemFieldSentencesOf(error, "en", {
        win_probability: "Win probability",
      }).get("win_probability"),
    ).toBe("Enter a whole number for Win probability.");
  });
});

describe("a whole number typed in a box", () => {
  const label = "Tokens per full user per month";
  const bounds = { min: 1, max: 1e12 };
  const fault = (typed: string) => wholeNumberFault(typed, "en", label, bounds);

  it("takes a whole number in range, with spaces around it", () => {
    expect(fault("1")).toBeUndefined();
    expect(fault(" 5 ")).toBeUndefined();
    expect(fault("1000000000000")).toBeUndefined();
  });

  it("says a fraction, a word or a comma is not a whole number", () => {
    for (const typed of ["1.5", "12,5", "abc", ""]) {
      expect(fault(typed)).toBe(
        "Enter a whole number for Tokens per full user per month.",
      );
    }
  });

  it("names the range a whole number falls outside", () => {
    for (const typed of ["0", "-1", "1000000000001"]) {
      expect(fault(typed)).toBe(
        "Tokens per full user per month must be from 1 to 1,000,000,000,000.",
      );
    }
  });
});
