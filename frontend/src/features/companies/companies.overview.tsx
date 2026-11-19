import { MergedIntoSentence } from "../../components/historymergedref";
import { Callout } from "../../design-system/callout";
import { useT } from "../../i18n";
import { type CompanyOverviewProps, CompanyReading } from "./companyreading";
import "./company360.css";

// The Overview tab: the Next bar, How it stands and the four cards under it,
// and nothing else. The account's other readings sit on the tab the design
// gives them (Tasks, Contacts, Profile).
export function CompanyOverviewStack(props: CompanyOverviewProps) {
  const t = useT();
  const { archived_at, merged_into_id } = props.company;
  return (
    <div className="co-overview-stack">
      {archived_at ? (
        // The server refuses an archived account's readings. So the tab every
        // link lands on says why it is bare and, after a merge, where to go.
        <Callout kind="standing" title={t("co.archived.overview")}>
          {merged_into_id && (
            <MergedIntoSentence kind="company" id={merged_into_id} />
          )}
        </Callout>
      ) : (
        <CompanyReading {...props} />
      )}
    </div>
  );
}
