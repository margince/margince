import { Sparkles } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Badge, Button } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { identifierNumber } from "../../format/format";
import { useT } from "../../i18n";
import { useOpenEmail } from "../../lib/openemail";
import { REWRITES, type RewriteVerb } from "../../lib/rewrites";
import { useViewerZone } from "../../lib/viewerzone";
import { openCitation } from "../records/record360";
import { VoiceDegradedNotice } from "./compose.notices";
import { DraftReasons } from "./composedraftcontext";
import { OpenEmailDrawer } from "./openemaildrawer";

type EmailDraft = components["schemas"]["EmailDraft"];
type VoiceProfile = components["schemas"]["VoiceProfile"];

// What the AI band says about a draft nobody typed, and the rewrites offered
// under it. Extracted from compose.tsx unchanged.
//
// The two belong together: the band discloses that a machine wrote the words,
// and a rewrite asks that machine to write them. A rewrite of the rep's own
// words fills the body with model words, so the disclosure follows it.

type DraftProvenance = Pick<
  EmailDraft,
  "ai_generated" | "ai_disclosure" | "voice_profile_version" | "voice_degraded"
>;

// Keep the server's disclosure verbatim, with a fallback when it is missing.
// One line at the head of the band, then the rep's verbs on the machine's
// words, then what the draft was written from.
export function DraftBand({
  provenance,
  maturity,
  reasons,
  actions,
}: Readonly<{
  provenance: DraftProvenance;
  maturity: VoiceProfile["maturity"] | undefined;
  reasons: components["schemas"]["AccountDraftReason"][];
  /** The rewrites and the discard, beside the disclosure they answer. */
  actions?: ReactNode;
}>) {
  const t = useT();
  // One shared email drawer keeps sources from opening competing dialogs.
  const [openEmail, setOpenEmail] = useOpenEmail();
  const zone = useViewerZone();
  if (!provenance.ai_generated) {
    return null;
  }
  return (
    <div className="compose-band-body">
      <div className="compose-band-line">
        <Sparkles aria-hidden="true" />
        <p className="compose-band-text">
          <Heading as="span" size="xsmall" role="heading" aria-level={3}>
            {t("compose.aiDisclosureTitle")}
          </Heading>{" "}
          <span className="t-caption">
            {provenance.ai_disclosure || t("compose.aiDisclosureFallback")}
            {provenance.voice_profile_version != null && (
              <>
                {" · "}
                {/* A profile VERSION, never grouped: version 1234 is one
                    identifier, and "1.234" reads as a different one. */}
                <span>
                  {t("compose.voiceVersion", {
                    n: identifierNumber(provenance.voice_profile_version),
                  })}
                </span>
              </>
            )}
          </span>
        </p>
        {actions && <ActionRow>{actions}</ActionRow>}
      </div>
      <DraftReasons
        reasons={reasons}
        onOpenRecord={openCitation}
        onOpenEmail={setOpenEmail}
      />
      <OpenEmailDrawer
        activityId={openEmail}
        zone={zone}
        onClose={() => setOpenEmail(null)}
      />
      <VoiceDegradedNotice degraded={provenance.voice_degraded} />
      {provenance.voice_profile_version != null &&
        maturity === "provisional" && (
          <p className="t-caption">
            <Badge>{t("compose.provisional")}</Badge>{" "}
            {t("compose.provisionalHint")}
          </p>
        )}
    </div>
  );
}

// Offered over any body on screen, whoever wrote it: the call revises the
// words sent as `rewrite_of`, and the editor's undo brings the old ones back.
// `refusal` names why this body cannot be sent for revision.
// `disabled` and not `pending`: these buttons do not report a write of their
// own, they refuse to start a second one. Named for what it does, because named
// for a state it does not have it read as the draft button's own spinner and a
// change to that button silently unblocked these.
export function RewriteRow({
  onRewrite,
  disabled,
  refusal = null,
}: Readonly<{
  onRewrite: (verb: RewriteVerb) => void;
  disabled: boolean;
  refusal?: string | null;
}>) {
  const t = useT();
  const refusalId = useId();
  return (
    <fieldset
      className="compose-rewrite"
      aria-describedby={refusal ? refusalId : undefined}
    >
      <legend className="sr-only">{t("compose.rewrite")}</legend>
      {REWRITES.map((rewrite) => (
        <Button
          key={rewrite.key}
          variant="aiQuiet"
          disabled={disabled || refusal !== null}
          onClick={() => onRewrite(rewrite.key)}
        >
          {t(rewrite.label)}
        </Button>
      ))}
      {refusal && (
        <p id={refusalId} className="t-caption">
          {refusal}
        </p>
      )}
    </fieldset>
  );
}
