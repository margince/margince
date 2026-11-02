import { describe, expect, it } from "vitest";
import { type Translator, translate } from "../i18n";
import { summaryWords } from "../lib/approvalpreviewfields";
import { noticeHeadline } from "./noticeheadline";

const t: Translator = (key, params) => translate("en", key, params);
const label = summaryWords(t).field;

describe("noticeHeadline", () => {
  it("heads an approval notice with its sentence and lists the values under it", () => {
    expect(
      noticeHeadline(
        {
          kind: "approval_pending",
          subject: "Update record deal: name=Acme, Inc., stage=won",
          summary_parts: {
            sentence: "Update record deal",
            fields: [
              { field: "name", value: "Acme, Inc." },
              { field: "stage", value: "won" },
            ],
            more_fields: 2,
          },
        },
        t,
      ),
    ).toEqual({
      headline: "Update record deal",
      detail: `${label("name")}: Acme, Inc. · ${label("stage")}: won · ${summaryWords(t).more(2)}`,
    });
  });

  // A value that holds ", " is the case a pattern over the joined text could
  // not read. The parts carry it whole.
  it("keeps a value with a comma in it as one value", () => {
    const { detail } = noticeHeadline(
      {
        kind: "approval_pending",
        subject: "Create company: name=Smith, Jones & Co",
        summary_parts: {
          sentence: "Create company",
          fields: [{ field: "name", value: "Smith, Jones & Co" }],
          more_fields: 0,
        },
      },
      t,
    );
    expect(detail).toBe(`${label("name")}: Smith, Jones & Co`);
  });

  it("heads a notice written before the parts were kept by its whole subject", () => {
    const subject = "Update record deal: name=Acme";
    expect(noticeHeadline({ kind: "approval_pending", subject }, t)).toEqual({
      headline: subject,
    });
  });

  it("names the class when an approval notice has no words of its own", () => {
    expect(
      noticeHeadline({ kind: "approval_pending", subject: "" }, t).headline,
    ).toBe(t("notifications.class.approval_pending.label"));
  });

  it("heads any other notice with its subject", () => {
    expect(
      noticeHeadline({ kind: "lead_sla", subject: "A lead waits" }, t),
    ).toEqual({ headline: "A lead waits" });
  });
});
