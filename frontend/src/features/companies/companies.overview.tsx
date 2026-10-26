import { type CompanyOverviewProps, CompanyReading } from "./companyreading";
import "./company360.css";

// The Overview tab: the Next bar, How it stands and the four cards under it,
// and nothing else. The account's other readings sit on the tab the design
// gives them (Tasks, Contacts, Profile).
export function CompanyOverviewStack(props: CompanyOverviewProps) {
  return (
    <div className="co-overview-stack">
      {!props.company.archived_at && <CompanyReading {...props} />}
    </div>
  );
}
