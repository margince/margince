import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { partnerFields } from "./deals.fields";

const t = (key: MessageKey) => translate("en", key);

describe("the partner fields on a deal's forms", () => {
  it("stand with the admin hint once the server says no company is a partner", () => {
    const fields = partnerFields(t, [], { noneYet: true });

    expect(fields.map((field) => field.key)).toEqual(["partner_company_id"]);
    expect(fields[0].options).toEqual([
      { value: "", label: t("deal.partnerNoneYet") },
    ]);
  });

  it("stay off the form while that answer is unknown", () => {
    expect(partnerFields(t, [])).toEqual([]);
  });

  it("offer the partners and the attribution that follows one", () => {
    const fields = partnerFields(t, [{ value: "p-1", label: "Xentral" }], {
      noneYet: false,
    });

    expect(fields.map((field) => field.key)).toEqual([
      "partner_company_id",
      "partner_attribution",
    ]);
    expect(fields[1].showWhen?.({ partner_company_id: "p-1" })).toBe(true);
    expect(fields[1].showWhen?.({ partner_company_id: "" })).toBe(false);
  });

  it("name a withheld partner as withheld, whatever the installation holds", () => {
    const fields = partnerFields(t, [], { withheld: true, noneYet: true });

    expect(fields[0].options).toEqual([
      { value: "", label: t("deal.partnerWithheld") },
    ]);
  });
});
