// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Sparkles } from "lucide-react";
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
 * The editor's own AI verb, at the head of its toolbar: it opens the ask for a
 * draft in the band under the bar. A toggle, so the band is the reader's to
 * open and to put away.
 */
export function AiDraftToggle({
  open,
  onToggle,
}: Readonly<{ open: boolean; onToggle: () => void }>) {
  const t = useT();
  return (
    <Button variant="aiQuiet" aria-expanded={open} onClick={onToggle}>
      <Sparkles aria-hidden="true" />
      {t("compose.draftWithAi")}
    </Button>
  );
}

/**
 * The band between the toolbar and the words: what the reader asks the model
 * for, the draft being written, and what the model wrote.
 *
 * The ask and the written draft can stand together. A reader who wants the
 * draft again with another steer opens the ask over the band that discloses
 * the first. Nothing renders when there is nothing to say, so the words sit
 * right under the bar.
 */
export function AiDraftBand({
  open,
  replying,
  source,
  fields,
  voice,
  voiceMaturity,
}: Readonly<{
  open: boolean;
  replying: boolean;
  /** What a draft is written from, said under the ask. */
  source: string;
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
  const busy = voice.control.pending && !open;
  if (!open && !drafted && !busy) {
    return null;
  }
  return (
    <div className="compose-aiband">
      {open && (
        <>
          <DraftOffer
            replying={replying}
            intent={fields.intent}
            onIntentChange={fields.setIntent}
            draft={voice.control}
            unavailable={fields.draftUnavailable}
          />
          <p className="t-caption">{source}</p>
        </>
      )}
      {busy && <p role="status">{t("compose.drafting")}</p>}
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
