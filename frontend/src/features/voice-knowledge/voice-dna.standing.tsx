import { Badge } from "../../design-system/atoms";
import { identifierNumber } from "../../format/format";
import { useT } from "../../i18n";
import type { VoiceProfile } from "./voice-dna";

// The profile's standing at the end of the jump bar. A state shows only when
// it wants something, since ready is normal. The version follows.
export function VoiceStanding({
  profile,
}: Readonly<{ profile: VoiceProfile }>) {
  const t = useT();
  return (
    <span className="vdna-status">
      {profile.status !== "ready" && (
        <Badge>{t(`settings.voice.status.${profile.status}`)}</Badge>
      )}
      {/* Version 0 is a profile no build has written yet: nothing to name. */}
      {Boolean(profile.profile_version) && (
        <span className="t-caption">
          {t("settings.voice.version", {
            n: identifierNumber(profile.profile_version ?? 0),
          })}
        </span>
      )}
    </span>
  );
}

export function DerivedVoice({ profile }: Readonly<{ profile: VoiceProfile }>) {
  const t = useT();
  return profile.voice_profile_md ? (
    <p className="vdna-derived">{profile.voice_profile_md}</p>
  ) : (
    <p>{t("settings.voice.derivedEmpty")}</p>
  );
}
