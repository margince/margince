// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { ActionRow } from "../design-system/actionrow";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { EntityKind } from "../lib/entity";
import { problemCodeOf, problemFieldErrorsOf } from "../lib/problem";
import { POLISH, type RewriteAsk, rewriteWords } from "../lib/rewrites";

export type PolishedWords = Readonly<{
  subject: string;
  body: string;
  aiAssisted?: boolean;
}>;

// The server's refusals that say why Polish cannot run here, so the button
// stays and says it instead of failing on every press. `words` refusals hold
// only for the words that were sent.
const REFUSALS: Readonly<
  Record<string, Readonly<{ key: MessageKey; words: boolean }>>
> = {
  ai_not_configured: { key: "log.polishNoModel", words: false },
  subject_restricted: { key: "log.polishRestricted", words: false },
  too_long: { key: "log.polishTooLong", words: true },
};

/**
 * Polish wording over the title and details a rep typed, on the record the
 * form is on. Undo puts the rep's words back while the polished ones are
 * untouched; the AI-assisted mark goes with them.
 */
export function PolishWording({
  words,
  onChange,
  offered,
  entityType,
  entityId,
}: Readonly<{
  words: PolishedWords;
  onChange: (next: PolishedWords) => void;
  // False over a transcript, which is kept byte for byte.
  offered: boolean;
  entityType?: EntityKind;
  entityId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The rep's words before the last polish, and the polish that replaced them.
  const [undo, setUndo] = useState<{
    before: PolishedWords;
    after: PolishedWords;
  } | null>(null);
  const polish = useMutation({
    mutationKey: ["activity-polish", entityId],
    mutationFn: ({ ask }: { ask: RewriteAsk; before: PolishedWords }) =>
      rewriteWords(ask, t),
    onSuccess: (polished, { before }) => {
      const after = { ...polished, aiAssisted: true };
      setUndo({ before, after });
      onChange(after);
    },
  });
  // The rewrite path names the record the words are on, so a form on no
  // record offers no Polish.
  if (
    !offered ||
    !entityType ||
    !entityId ||
    (!words.subject.trim() && !words.body.trim())
  ) {
    return null;
  }
  const refusal = refusalOf(polish.error, polish.variables?.before, words, t);
  const undoable =
    undo !== null &&
    undo.after.subject === words.subject &&
    undo.after.body === words.body;
  return (
    <>
      <ActionRow>
        <Button
          variant="aiQuiet"
          pending={polish.isPending}
          busyLabel={t("log.polishPending")}
          reason={refusal}
          onClick={() =>
            polish.mutate({
              before: {
                subject: words.subject,
                body: words.body,
                aiAssisted: words.aiAssisted === true,
              },
              ask: {
                envelope: "note",
                verb: POLISH.key,
                subject: words.subject,
                body: words.body,
                language: locale,
                about: { entity_type: entityType, entity_id: entityId },
              },
            })
          }
        >
          {t("log.polish")}
        </Button>
        {undoable && (
          <Button
            onClick={() => {
              onChange(undo.before);
              setUndo(null);
            }}
          >
            {t("common.undo")}
          </Button>
        )}
        {words.aiAssisted && <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>}
      </ActionRow>
      {polish.isError && !refusal && <ErrorLine error={polish.error} />}
    </>
  );
}

function refusalOf(
  error: unknown,
  sent: PolishedWords | undefined,
  words: PolishedWords,
  t: ReturnType<typeof useT>,
): string | undefined {
  // A length refusal is a 422 whose field names the code.
  const tooLong = problemFieldErrorsOf(error).some(
    (f) => f.code === "too_long",
  );
  const code = tooLong ? "too_long" : problemCodeOf(error);
  const refusal = code === null ? undefined : REFUSALS[code];
  if (!refusal) return undefined;
  const changed = sent?.subject !== words.subject || sent?.body !== words.body;
  return refusal.words && changed ? undefined : t(refusal.key);
}
