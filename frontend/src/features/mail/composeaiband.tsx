// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { DraftBand, RewriteRow } from "./composedraftband";
import { DraftOffer } from "./composedraftcontext";
import type { ComposeFields } from "./composefields";
import type { VoiceDraft } from "./composevoicedraft";

type VoiceProfile = components["schemas"]["VoiceProfile"];

/**
 * The line above the words that a model writes through: the ask for a draft,
 * the draft being written, then what the model wrote.
 *
 * The ask stays, so a changed steer can ask again. Once a draft stands, the
 * band also discloses it and offers the rewrites and the discard.
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
  const drafted = provenance?.ai_generated === true;
  // A rewrite replaces the body, so it is offered over the model's own
  // untouched words. A draft in flight blocks it too.
  const untouched = fields.body !== "" && fields.body === fields.servedBody;
  const discard = voice.discardControl;
  return (
    <div className="compose-aiband">
      <DraftOffer
        replying={replying}
        intent={fields.intent}
        onIntentChange={fields.setIntent}
        draft={voice.control}
        unavailable={fields.draftUnavailable}
      />
      {drafted && provenance && (
        <DraftBand
          provenance={provenance}
          maturity={voiceMaturity}
          reasons={account.reasoning}
          actions={
            (untouched || discard) && (
              <div className="compose-aiband-actions">
                {untouched && (
                  <RewriteRow
                    disabled={voice.control.pending || voice.control.disabled}
                    onRewrite={voice.rewrite}
                  />
                )}
                {discard && (
                  <Button
                    variant="ghost"
                    onClick={discard.run}
                    pending={discard.pending}
                    disabled={discard.disabled}
                    // Discarding tells the voice profile this draft missed; it
                    // is not an undo for clearing the box.
                    title={t("compose.discardDraftHint")}
                  >
                    {t("compose.discardDraft")}
                  </Button>
                )}
              </div>
            )
          }
        />
      )}
      {discard?.error && <ErrorLine>{discard.error}</ErrorLine>}
    </div>
  );
}
