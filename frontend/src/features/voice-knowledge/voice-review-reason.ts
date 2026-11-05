import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import type { Locale, useT } from "../../i18n";

function plainVoiceReason(
  t: ReturnType<typeof useT>,
  code: string,
): string | undefined {
  switch (code) {
    case "malformed":
      return t("voice.candidate.reason.malformed");
    case "uncomparable":
      return t("voice.candidate.reason.uncomparable");
    case "too_small":
      return t("voice.candidate.reason.smallCorpus");
    case "corpus_changed":
      return t("voice.candidate.reason.corpusChanged");
    case "active_changed":
      return t("voice.candidate.reason.activeChanged");
    default:
      return undefined;
  }
}

function typedReviewReasonText(
  t: ReturnType<typeof useT>,
  locale: Locale,
  detail: components["schemas"]["VoiceReviewReason"],
): string | undefined {
  const { code } = detail;
  const values = detail.values ?? {};
  const finite = (value: number | undefined): value is number =>
    typeof value === "number" && Number.isFinite(value);
  if (code === "low_score" && finite(values.score) && finite(values.floor)) {
    return t("voice.candidate.reason.lowScore", {
      score: formatNumber(values.score, locale),
      floor: formatNumber(values.floor, locale),
    });
  }
  if (code === "hard_failures" && finite(values.count)) {
    return t("voice.candidate.reason.hardFailures", {
      n: formatNumber(values.count, locale),
    });
  }
  if (
    code === "rules_removed" &&
    finite(values.avoid) &&
    finite(values.register)
  ) {
    return t("voice.candidate.reason.rulesRemoved", {
      n: formatNumber(values.avoid + values.register, locale),
    });
  }
  const plain = plainVoiceReason(t, code);
  if (plain) return plain;
  if (code === "identity_shift" && finite(values.score)) {
    return t("voice.candidate.reason.identity", {
      score: formatNumber(values.score, locale),
    });
  }
  if (code === "signature_shift" && finite(values.score)) {
    return t("voice.candidate.reason.signature", {
      score: formatNumber(values.score, locale),
    });
  }
  if (code === "regression" && finite(values.score) && finite(values.active)) {
    return t("voice.candidate.reason.regression", {
      score: formatNumber(values.score, locale),
      active: formatNumber(values.active, locale),
    });
  }

  return undefined;
}

function legacyReviewReason(
  reason: string,
): components["schemas"]["VoiceReviewReason"] | undefined {
  const lowScore =
    /median voice score ([\d.]+) is below the ([\d.]+) floor/.exec(reason);
  if (lowScore)
    return {
      code: "low_score",
      text: reason,
      values: { score: Number(lowScore[1]), floor: Number(lowScore[2]) },
    };
  const hardFailures = /^(\d+) anti-AI hard failures survived/.exec(reason);
  if (hardFailures)
    return {
      code: "hard_failures",
      text: reason,
      values: { count: Number(hardFailures[1]) },
    };
  const rulesRemoved =
    /^(\d+) avoid and (\d+) register rules were removed/.exec(reason);
  if (rulesRemoved)
    return {
      code: "rules_removed",
      text: reason,
      values: {
        avoid: Number(rulesRemoved[1]),
        register: Number(rulesRemoved[2]),
      },
    };
  if (reason.includes("malformed drafts during evaluation"))
    return { code: "malformed", text: reason, values: {} };
  return undefined;
}

export function reviewReasonText(
  t: ReturnType<typeof useT>,
  locale: Locale,
  reason: string,
  detail?: components["schemas"]["VoiceReviewReason"] | null,
): string {
  if (detail !== undefined) {
    return detail?.text === reason
      ? (typedReviewReasonText(t, locale, detail) ?? reason)
      : reason;
  }
  const legacy = legacyReviewReason(reason);
  return (legacy && typedReviewReasonText(t, locale, legacy)) ?? reason;
}
