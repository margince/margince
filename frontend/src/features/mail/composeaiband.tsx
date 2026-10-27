// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { RefreshCw, RotateCcw } from "lucide-react";
import { useId, useState } from "react";
import type { components } from "../../api/schema";
import { ErrorLine } from "../../design-system/errorline";
import { IconAction } from "../../design-system/iconaction";
import { useT } from "../../i18n";
import { DraftBand, RewriteRow } from "./composedraftband";
import { DraftOffer } from "./composedraftcontext";
import type { ComposeFields } from "./composefields";
import type { VoiceDraft } from "./composevoicedraft";

type VoiceProfile = components["schemas"]["VoiceProfile"];

/**
 * The line above the words that a model writes through: the ask for a draft.
 * Once a draft stands, one line discloses it and holds its rewrites, the ask
 * again (folded, so a changed steer can redraft) and the discard.
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
  const control = voice.control;
  const offerId = useId();
  // Held per draft: a new draft is a new provenance, which folds the ask away.
  const [askingOver, setAskingOver] = useState<typeof provenance>(null);
  const offer = (
    <DraftOffer
      replying={replying}
      intent={fields.intent}
      onIntentChange={fields.setIntent}
      draft={control}
      unavailable={fields.draftUnavailable}
      need={voice.need}
    />
  );
  if (!drafted || !provenance) {
    return <div className="compose-aiband">{offer}</div>;
  }
  const asking = askingOver === provenance;
  return (
    <div className="compose-aiband">
      <DraftBand
        provenance={provenance}
        maturity={voiceMaturity}
        reasons={account.reasoning}
        actions={
          <>
            {untouched && (
              <RewriteRow
                disabled={control.pending || control.disabled}
                onRewrite={voice.rewrite}
              />
            )}
            <IconAction
              label={t("compose.askAgain")}
              icon={<RefreshCw aria-hidden="true" />}
              onClick={() => setAskingOver(asking ? null : provenance)}
              disclosure={{ expanded: asking, controls: offerId }}
            />
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
      {asking ? (
        <div id={offerId}>{offer}</div>
      ) : (
        control.error && <ErrorLine>{control.error}</ErrorLine>
      )}
      {discard?.error && <ErrorLine>{discard.error}</ErrorLine>}
    </div>
  );
}
