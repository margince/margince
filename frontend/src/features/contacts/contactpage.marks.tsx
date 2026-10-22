import type { ReactNode } from "react";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { dealRoleLabel } from "../../lib/recordlabels";
import { RecordAccess } from "../records/recordaccess";
import type { Contact360 } from "./contactpage";
import { awaitsOurReply } from "./contactstanding";

// The pills under the name: whose move it is, the buying role, and who may
// read the record. How the relationship stands is How it stands' to say.
export function ContactMarks({
  view,
}: Readonly<{ view: Contact360 }>): ReactNode {
  const t = useT();
  const role = view.commercial?.role;
  return (
    <>
      {awaitsOurReply(view) && (
        <Badge tone="warning" dot>
          {t("standing.yourMove")}
        </Badge>
      )}
      {role && <Badge>{dealRoleLabel(role, t)}</Badge>}
      <RecordAccess
        key={view.contact.id}
        kind="contact"
        record={view.contact}
      />
    </>
  );
}
