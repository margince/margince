import type { components } from "../../api/schema";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { QueryStates } from "../../design-system/querystates";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useAutomationPreview } from "./automations.previewquery";

export function ScheduledPopulationReview({
  automation,
  params,
  pending,
  error,
  onConfirm,
  onClose,
}: Readonly<{
  automation: components["schemas"]["Automation"];
  params?: Record<string, unknown>;
  pending: boolean;
  error: string | null;
  onConfirm: () => void;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const preview = useAutomationPreview({
    id: automation.id,
    version: automation.version,
    params,
  });
  return (
    <ConfirmModal
      open
      title={t("auto.initialRun.title")}
      confirmLabel={t("auto.initialRun.confirm")}
      confirmDisabled={
        !preview.isSuccess ||
        preview.isFetching ||
        preview.data.matches_now_available !== true
      }
      pending={pending}
      error={error}
      onConfirm={onConfirm}
      onClose={onClose}
    >
      <p>{t("auto.initialRun.body", { name: automation.name })}</p>
      <QueryStates query={preview} pendingLabel={t("auto.preview.title")}>
        {preview.data && (
          <p>
            {t("auto.preview.matchesNow", {
              n: formatNumber(preview.data.matches_now, locale),
            })}
          </p>
        )}
      </QueryStates>
    </ConfirmModal>
  );
}
