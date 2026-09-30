import type { ReactNode } from "react";
import { navigate } from "../app/router";
import { BoughtMark, BoughtText, boughtFields } from "./boughtmarks";
import type { Contact360 } from "./contact360";
import { currentEmployer } from "./employmentcurrency";

// The header's second line: what this contact does, and where. The company is a
// link because it is a record of its own, not a label. A title a purchase filled
// is underlined as bought; the employer is a button, so its mark sits beside it.
export function ContactSubtitle({
  view,
}: Readonly<{ view: Contact360 }>): ReactNode {
  const contact = view.contact;
  const bought = boughtFields(contact);
  const employment = currentEmployer(view.employments?.data);
  return (
    <div className="record-sub record-sub-inline">
      {contact.title && (
        <BoughtText value={contact.title} bought={bought.get("title")} />
      )}
      {employment?.company_name && (
        <>
          {contact.title ? " · " : ""}
          <button
            type="button"
            className="pe-meta-link"
            onClick={() =>
              navigate({
                screen: "companies",
                id: employment.company_id,
              })
            }
          >
            {employment.company_name}
          </button>
          <BoughtMark
            bought={bought.get(`employment:${employment.relationship_id}`)}
            subject={employment.company_name}
          />
        </>
      )}
    </div>
  );
}
