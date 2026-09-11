import { Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody } from "../design-system/panel";
import { identifierNumber } from "../format/format";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { VoiceDegradedNotice } from "./compose.notices";
import { DraftReasons, openCited } from "./composedraftcontext";
import { useOpenEmail } from "./openemail";

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

// The card that says a MACHINE wrote the words below, and what it wrote them
// from: to a reader the Art. 50 disclosure and the draft's reasoning are one
// statement — not your colleague's message, and here is what it stands on.
//
// `Panel tone="ai"` draws it, in the colour every other machine-authored
// surface wears, its title at h3 under the drawer's own h2. Loudest thing in
// the drawer on purpose: miss it and a model's words go out in a rep's name.
//
// The server's disclosure line is a compliance string rendered verbatim, never
// reworded; a response that omits it still discloses, because a missing line
// may not silently become a missing disclosure.
//
// The voice tag names the PROFILE version that styled the draft; the
// provisional label reports what that profile is today. Neither implies a
// weaker draft — nothing gates drafting on maturity. Both hang off the SERVED
// version, because reporting a maturity over a draft no voice touched would
// overstate this surface's own provenance, which Art. 50 does not permit.
export function DraftBand({
  provenance,
  maturity,
  reasons,
  children,
}: Readonly<{
  provenance: DraftProvenance;
  maturity: VoiceProfile["maturity"] | undefined;
  reasons: components["schemas"]["AccountDraftReason"][];
  // The steer and the verb that asks for another draft: the card is the
  // machine's own block, so asking it to write again belongs inside it.
  children: ReactNode;
}>) {
  const t = useT();
  // One drawer per CARD rather than per reason: a drawer per row would be
  // several dialogs racing to be the one on top.
  const [openEmail, setOpenEmail] = useOpenEmail();
  const zone = useRecordZone();
  if (!provenance.ai_generated) {
    return null;
  }
  return (
    <Panel
      tone="ai"
      title={t("compose.aiDisclosureTitle")}
      titleLevel={3}
      className="compose-band"
    >
      <PanelBody className="compose-band-body">
        <p className="t-body">
          {provenance.ai_disclosure || t("compose.aiDisclosureFallback")}
        </p>
        <DraftReasons
          reasons={reasons}
          onOpenRecord={openCited}
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
              <p className="t-caption">
                <Badge>{t("compose.provisional")}</Badge>{" "}
                {t("compose.provisionalHint")}
              </p>
            )}
          </>
        )}
        {children}
      </PanelBody>
    </Panel>
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
          small
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
