import { DealStatusBadge } from "../../components/dealstatusbadge";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { Deal } from "./deals";

// The pills under the deal's name. They say whose move it is and when a buyer
// is owed a reply. Then come the deal's status and, as on the board card,
// whether it is archived.
export function DealPulsePills({
  status,
  archived,
  yourMove,
}: Readonly<{ status: Deal["status"]; archived: boolean; yourMove: boolean }>) {
  const t = useT();
  return (
    <>
      {yourMove && (
        <Badge tone="warning" dot>
          {t("deal.standing.teamReply")}
        </Badge>
      )}
      <DealStatusBadge status={status} />
      {archived && <Badge>{t("deal.archived")}</Badge>}
    </>
  );
}
