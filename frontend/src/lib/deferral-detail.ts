import type { Translator } from "../i18n";

type DeferredOperation = Readonly<{
  status: string;
  deferral_reason?: string;
  status_detail?: string | null;
}>;

export function deferralDetail(
  t: Translator,
  operation: DeferredOperation,
): string | null {
  if (operation.status === "deferred") {
    if (operation.deferral_reason === "provider") return t("aiWait.provider");
    if (operation.deferral_reason === "budget") return t("aiWait.budget");
  }
  return operation.status_detail ?? null;
}
