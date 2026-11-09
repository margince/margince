import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { useLocale, usePlural, useT } from "../../i18n";
import {
  usePatchSchedulingProfile,
  useSchedulingProfile,
} from "../../lib/scheduling-profile-query";
import { currentBookingAddress } from "./booking-alias";
import { hoursSentence } from "./meeting-phrases";
import { useWorkingHours } from "./working-hours";
/** The hours a booking through this link is offered in, in one sentence. */
export function HoursSummary({
  notice,
  onChangeHours,
}: Readonly<{ notice: number; onChangeHours: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const hours = useWorkingHours().data?.working_hours;
  if (!hours) return null;
  return (
    <Callout
      title={hoursSentence(t, plural, locale, hours, notice)}
      actions={
        <Button variant="link" onClick={onChangeHours}>
          {t("scheduling.changeHours")}
        </Button>
      }
    />
  );
}

/** Replacing the link's address, asked first: the old address stops working. */
export function ReplaceAddressConfirm({
  open,
  onClose,
}: Readonly<{ open: boolean; onClose: () => void }>) {
  const t = useT();
  const replace = usePatchSchedulingProfile();
  // Read from the cache the settings page already holds.
  const profile = useSchedulingProfile().data;
  return (
    <ConfirmModal
      open={open}
      onClose={() => {
        replace.reset();
        onClose();
      }}
      title={t("scheduling.replaceConfirmTitle")}
      confirmLabel={t("scheduling.replaceAddress")}
      pending={replace.isPending}
      onConfirm={() =>
        replace.mutate({ replace_link: true }, { onSuccess: onClose })
      }
    >
      <p>{t("scheduling.replaceHelp")}</p>
      {profile?.alias && (
        <p>
          {t("scheduling.replaceAliasHelp", {
            url: currentBookingAddress(profile, profile.alias),
          })}
        </p>
      )}
      <ErrorLine error={replace.error} />
    </ConfirmModal>
  );
}
