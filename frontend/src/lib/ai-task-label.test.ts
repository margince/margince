import { describe, expect, it } from "vitest";
import { type Locale, type Translator, translate } from "../i18n";
import { aiTaskLabel } from "./ai-task-label";

const cases: ReadonlyArray<{
  locale: Locale;
  name: string;
  rewrite: string;
  unknown: string;
}> = [
  {
    locale: "en",
    name: "Website triage",
    rewrite: "Rewriting",
    unknown: "AI task",
  },
  {
    locale: "de",
    name: "Website-Prüfung",
    rewrite: "Umformulieren",
    unknown: "KI-Aufgabe",
  },
  {
    locale: "vi",
    name: "Đánh giá trang web",
    rewrite: "Viết lại",
    unknown: "Tác vụ AI",
  },
];

describe.each(cases)(
  "AI task names in $locale",
  ({ locale, name, rewrite, unknown }) => {
    const t: Translator = (key, params) => translate(locale, key, params);

    it("uses the reader's language for a known task", () => {
      expect(aiTaskLabel("site_triage", t, "Website triage")).toBe(name);
    });

    it("names the rewrite verbs' own task, which the routing settings list", () => {
      expect(aiTaskLabel("rewrite", t, "Rewriting")).toBe(rewrite);
    });

    it("names the separately routed embedding lane in the reader's language", () => {
      expect(aiTaskLabel("embeddings", t, "Search and retrieval")).toBe(
        t("aiRouting.lane.embeddings"),
      );
    });

    it("keeps a server-provided name for an unknown task", () => {
      expect(aiTaskLabel("future_task", t, "Future account review")).toBe(
        "Future account review",
      );
    });

    it("never substitutes a wire key for a missing name", () => {
      expect(aiTaskLabel("future_task", t)).toBe(unknown);
      expect(aiTaskLabel("future_task", t, "future_task")).toBe(unknown);
      expect(aiTaskLabel("constructor", t)).toBe(unknown);
      expect(aiTaskLabel(undefined, t, " ")).toBe(unknown);
    });
  },
);
