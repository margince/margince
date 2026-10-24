import type { ReactNode } from "react";
import { useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { BoughtMark, BoughtText, boughtFields } from "./boughtmarks";
import type { Contact360 } from "./contact360";
import { currentEmployer } from "./employmentcurrency";

// The lines under the name, one fact each: "{title} at" over the employer,
// which is a link because it is a record of its own. A title a purchase filled
// is underlined as bought; the employer is a button, so its mark sits beside it.
export function ContactSubtitle({
  view,
}: Readonly<{ view: Contact360 }>): ReactNode {
  const t = useT();
  const contact = view.contact;
  const bought = boughtFields(contact);
  const employment = currentEmployer(view.employments?.data);
  const company = employment?.company_name;
  return (
    <>
      {contact.title && (
        <span>
          <BoughtText value={contact.title} bought={bought.get("title")} />
          {company && ` ${t("contact.subtitle.at")}`}
        </span>
      )}
      {employment && company && (
        <span>
          <button
            type="button"
            className="entity-link"
            onClick={() =>
              navigate({
                screen: "companies",
                id: employment.company_id,
              })
            }
          >
            {company}
          </button>
          <BoughtMark
            bought={bought.get(`employment:${employment.relationship_id}`)}
            subject={company}
          />
        </span>
      )}
    </>
  );
}
