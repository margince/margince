import type { Translator } from "../i18n";

export function offerStatusLabel(status: string, t: Translator): string {
  switch (status) {
    case "draft":
    case "sent":
    case "accepted":
    case "rejected":
    case "expired":
    case "superseded":
      return t(`commercial.offer.${status}`);
    default:
      return status;
  }
}
