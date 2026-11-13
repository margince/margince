// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { RotateCcw, Undo2 } from "lucide-react";
import type { components } from "../../api/schema";
import { ErrorLine } from "../../design-system/errorline";
import { IconAction } from "../../design-system/iconaction";
import { LanguageSelect } from "../../design-system/languageselect";
import { useT } from "../../i18n";
import { isLocale, LOCALES, type Locale } from "../../i18n/locale";
import { REWRITE_MAX_CHARACTERS } from "../../lib/rewrites";
import { DraftBand, RewriteRow } from "./composedraftband";
import { rewriteBlock } from "./composedraftcall";
import { DraftOffer } from "./composedraftcontext";
import type { ComposeFields } from "./composefields";
import type { VoiceDraft } from "./composevoicedraft";

type VoiceProfile = components["schemas"]["VoiceProfile"];

/**
 * The line above the words that a model writes through: the ask for a draft.
 * Once a draft stands, the purpose it was written to stays on screen to be
 * corrected, and one line discloses the draft with its rewrites and discard.
 */
export function AiDraftBand({
  replying,
  fields,
  voice,
  voiceMaturity,
}: Readonly<{
  replying: boolean;
  fields: ComposeFields;
  voice: VoiceDraft;
  voiceMaturity: VoiceProfile["maturity"] | undefined;
}>) {
  const t = useT();
  const { provenance, account } = fields;
  const control = voice.control;
  const drafted = provenance?.ai_generated === true;
  const undo = voice.undoRewrite;
  const rewrites = fields.body.trim() !== "" && (
    <>
      <RewriteRow
        disabled={control.pending || control.disabled}
        refusal={rewriteRefusal(fields, t)}
        onRewrite={voice.rewrite}
      />
      {undo && (
        <IconAction
          label={t("common.undo")}
          icon={<Undo2 aria-hidden="true" />}
          onClick={undo}
        />
      )}
    </>
  );
  const discard = voice.discardControl;
  const shownLanguage = draftLanguage(fields);
  const languagePick = shownLanguage && (
    <DraftLanguagePick
      language={shownLanguage}
      onPick={voice.relanguage}
      pending={control.pending}
      disabled={control.pending || control.disabled}
    />
  );
  const offer = (
    <DraftOffer
      replying={replying}
      intent={fields.intent}
      onIntentChange={fields.setIntent}
      draft={control}
      unavailable={fields.draftUnavailable}
      need={voice.need}
      verb={offerVerb(
        drafted && provenance !== null,
        fields.intent !== fields.draftedIntent,
      )}
    />
  );
  if (!drafted || !provenance) {
    return (
      <div className="compose-aiband">
        {offer}
        {languagePick}
        {rewrites}
      </div>
    );
  }
  return (
    <div className="compose-aiband">
      {offer}
      <DraftBand
        provenance={provenance}
        maturity={voiceMaturity}
        reasons={account.reasoning}
        actions={
          <>
            {languagePick}
            {rewrites}
            {discard && (
              <IconAction
                // Discarding tells the voice profile this draft missed; it is
                // not an undo for clearing the box.
                label={t("compose.discardDraft")}
                hint={t("compose.discardDraftHint")}
                icon={<RotateCcw aria-hidden="true" />}
                onClick={discard.run}
                pending={discard.pending}
                disabled={discard.disabled}
              />
            )}
          </>
        }
      />
      {discard?.error && <ErrorLine>{discard.error}</ErrorLine>}
    </div>
  );
}

function offerVerb(
  standing: boolean,
  purposeChanged: boolean,
): "draft" | "redraft" | null {
  if (!standing) return "draft";
  return purposeChanged ? "redraft" : null;
}

function rewriteRefusal(
  fields: ComposeFields,
  t: ReturnType<typeof useT>,
): string | null {
  switch (
    rewriteBlock(
      { text: fields.body, html: fields.html },
      REWRITE_MAX_CHARACTERS,
    )
  ) {
    case "formatted":
      return t("compose.rewriteFormatted");
    case "long":
      return t("compose.rewriteTooLong");
    default:
      return null;
  }
}

// The rep's pick, else the language the server reported for the last draft
// that filled the body. A code this build does not ship offers no picker.
function draftLanguage(fields: ComposeFields): Locale | null {
  if (fields.language) return fields.language;
  const reported = fields.provenance?.language;
  return reported !== undefined && isLocale(reported) ? reported : null;
}

// Over edited words, the pick sends them to be written again in another
// language under the same ask.
function DraftLanguagePick({
  language,
  onPick,
  pending,
  disabled,
}: Readonly<{
  language: Locale;
  onPick: (next: Locale) => void;
  pending: boolean;
  disabled: boolean;
}>) {
  const t = useT();
  return (
    <LanguageSelect
      className="compose-draftlanguage"
      aria-label={t("compose.draftLanguage")}
      aria-busy={pending}
      languages={LOCALES}
      value={language}
      disabled={disabled}
      onChange={(next) => {
        if (next !== language) onPick(next);
      }}
    />
  );
}
