import { Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Eyebrow } from "../design-system/eyebrow";
import { Heading } from "../design-system/heading";
import { identifierNumber } from "../format/format";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useOpenEmail } from "../lib/openemail";
import { useRecordZone } from "../lib/recordzone";
import { VoiceDegradedNotice } from "./compose.notices";
import { DraftReasons } from "./composedraftcontext";
import { OpenEmailDrawer } from "./openemaildrawer";
import { openCitation } from "./record360";

type EmailDraft = components["schemas"]["EmailDraft"];
type VoiceProfile = components["schemas"]["VoiceProfile"];

// What the AI band says about a draft nobody typed, and the rewrites offered
// under it. Extracted from compose.tsx unchanged.
//
// The two belong together: the band discloses that a machine wrote the words,
// and every rewrite row asks that machine to write them again. A surface that
// showed one without the other would either disclose nothing or offer a rewrite
// of prose the reader believes they wrote themselves.

type DraftProvenance = Pick<
  EmailDraft,
  "ai_generated" | "ai_disclosure" | "voice_profile_version" | "voice_degraded"
>;

// Keep the server's disclosure verbatim, with a fallback when it is missing.
export function DraftBand({
  provenance,
  maturity,
  reasons,
  children,
}: Readonly<{
  provenance: DraftProvenance;
  maturity: VoiceProfile["maturity"] | undefined;
  reasons: components["schemas"]["AccountDraftReason"][];
  children: ReactNode;
}>) {
  const t = useT();
  // One shared email drawer keeps sources from opening competing dialogs.
  const [openEmail, setOpenEmail] = useOpenEmail();
  const zone = useRecordZone();
  if (!provenance.ai_generated) {
    return null;
  }
  return (
    <div className="compose-band-body">
      <Callout
        tone="ai"
        kind="standing"
        title={
          <Heading as="span" size="xsmall" role="heading" aria-level={3}>
            {t("compose.aiDisclosureTitle")}
          </Heading>
        }
      >
        {provenance.ai_disclosure || t("compose.aiDisclosureFallback")}
      </Callout>
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
      {provenance.voice_profile_version != null && (
        <>
          <p className="t-caption">
            {/* A profile VERSION, never grouped: version 1234 is one
                  identifier, and "1.234" reads as a different one. */}
            {t("compose.voiceVersion", {
              n: identifierNumber(provenance.voice_profile_version),
            })}
          </p>
          {maturity === "provisional" && (
            <p>
              <Badge>{t("compose.provisional")}</Badge>{" "}
              {t("compose.provisionalHint")}
            </p>
          )}
        </>
      )}
      {children}
    </div>
  );
}

// The four things a rep asks the machine to do to its own draft, as one press
// each. Each is an instruction for ONE call — it never becomes the standing
// steer in the intent field.
// The label a rep reads and the instruction the model is given are two
// different strings and both are translated: the button says "Shorter" and the
// model is asked for it in a sentence, because an instruction of one word is
// one the model has to guess the scope of.
export const REWRITES = [
  {
    key: "shorter",
    label: "compose.rewriteShorter",
    instruction: "compose.rewriteShorterAsk",
  },
  {
    key: "warmer",
    label: "compose.rewriteWarmer",
    instruction: "compose.rewriteWarmerAsk",
  },
  {
    key: "formal",
    label: "compose.rewriteFormal",
    instruction: "compose.rewriteFormalAsk",
  },
  {
    key: "deadline",
    label: "compose.rewriteDeadline",
    instruction: "compose.rewriteDeadlineAsk",
  },
] as const satisfies readonly {
  key: string;
  label: MessageKey;
  instruction: MessageKey;
}[];

// Offered only over the machine's OWN untouched words. Once the rep has
// edited the body, a rewrite would throw their work away to answer a question
// about text that is no longer there — so the row withdraws rather than
// growing a confirm nobody would read.
// `disabled` and not `pending`: these buttons do not report a write of their
// own, they refuse to start a second one. Named for what it does, because named
// for a state it does not have it read as the draft button's own spinner and a
// change to that button silently unblocked these.
export function RewriteRow({
  onRewrite,
  disabled,
}: Readonly<{
  onRewrite: (instruction: string) => void;
  disabled: boolean;
}>) {
  const t = useT();
  return (
    <div className="compose-rewrite">
      <Eyebrow>{t("compose.rewrite")}</Eyebrow>
      {REWRITES.map((rewrite) => (
        <Button
          key={rewrite.key}
          variant="aiQuiet"
          disabled={disabled}
          onClick={() => onRewrite(t(rewrite.instruction))}
        >
          <Sparkles aria-hidden="true" />
          {t(rewrite.label)}
        </Button>
      ))}
    </div>
  );
}
