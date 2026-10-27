import type { ReactNode } from "react";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { dealRoleLabel } from "../../lib/recordlabels";
import { RecordAccess } from "../records/recordaccess";
import type { Contact360 } from "./contactpage";

// Role and record access do not infer an obligation from exchange order.
export function ContactMarks({
  view,
}: Readonly<{ view: Contact360 }>): ReactNode {
  const t = useT();
  const role = view.commercial?.role;
  return (
    <>
      {role && <Badge>{dealRoleLabel(role, t)}</Badge>}
      <RecordAccess
        key={view.contact.id}
        kind="contact"
        record={view.contact}
      />
    </>
  );
}
