import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { ensureProfileId } from "../../lib/voice-profile";

// One press reads the owner's own sent mail into their samples. Nothing reads
// mail before the press, so the row states what the press reads and keeps.

type Learned = components["schemas"]["VoiceSentMailLearned"];

type Outcome =
  | Readonly<{ kind: "learned"; learned: Learned }>
  | Readonly<{ kind: "failed"; problem: unknown }>;

/** One press and what it did, owned by the card so the result outlives the
 * swap from the first-sample card to the full one. */
export type SentMailLearning = ReturnType<typeof useSentMailLearning>;

export function useSentMailLearning(onChanged: () => void) {
  const [reading, setReading] = useState(false);
  const [outcome, setOutcome] = useState<Outcome | null>(null);

  const press = async () => {
    setReading(true);
    setOutcome(null);
    try {
      // The first press mints the profile, as the first uploaded sample does.
      const profileId = await ensureProfileId();
      const { data, error } = await api.POST(
        "/voice-profiles/{id}/sources/sent-mail",
        { params: { path: { id: profileId } } },
      );
      if (error || !data) {
        setOutcome({ kind: "failed", problem: error });
        return;
      }
      setOutcome({ kind: "learned", learned: data });
      onChanged();
    } catch (problem: unknown) {
      setOutcome({ kind: "failed", problem });
    } finally {
      setReading(false);
    }
  };

  return { press, reading, outcome };
}

export function LearnFromSentMail({
  learning,
}: Readonly<{ learning: SentMailLearning }>) {
  const t = useT();
  const { press, reading, outcome } = learning;

  return (
    <SettingRow
      label={t("settings.voice.sentMailLabel")}
      description={t("settings.voice.sentMailConsent")}
      layout="stack"
      control={(control) => (
        <div className="vdna-zone">
          <div className="vdna-composer-actions">
            <Button
              {...control}
              disabled={reading}
              onClick={() => void press()}
            >
              {reading
                ? t("settings.voice.sentMailReading")
                : t("settings.voice.sentMailButton")}
            </Button>
          </div>
          <div aria-live="polite">
            {outcome && <OutcomeNotice outcome={outcome} />}
          </div>
        </div>
      )}
    />
  );
}

// What one press did, said in the order a reader asks: how many mails taught
// the voice, then why the rest did not.
function OutcomeNotice({ outcome }: Readonly<{ outcome: Outcome }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (outcome.kind === "failed") {
    return (
      <p className="vdna-notice-warning" role="alert">
        {t("settings.voice.sentMailFailed", {
          detail: problemMessageOf(outcome.problem, t),
        })}
      </p>
    );
  }
  const { read, added, no_own_words: noOwnWords } = outcome.learned;
  if (read === 0) {
    return (
      <p className="vdna-notice-warning">{t("settings.voice.sentMailNone")}</p>
    );
  }
  const count = (n: number) => ({ count: formatNumber(n, locale) });
  return (
    <ul className="vdna-notices">
      <li className={added > 0 ? "vdna-notice-ok" : "vdna-notice-warning"}>
        {added > 0
          ? plural("settings.voice.sentMailLearned", added, count(added))
          : t("settings.voice.sentMailNothingNew")}
      </li>
      {noOwnWords > 0 && (
        <li className="vdna-notice-warning">
          {plural(
            "settings.voice.sentMailNoOwnWords",
            noOwnWords,
            count(noOwnWords),
          )}
        </li>
      )}
    </ul>
  );
}
