import type { components } from "../../api/schema";
import { PendingBody } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import { BoughtText, boughtFields } from "./boughtmarks";
import { useCachedRecordName } from "./recordidentity";

type Contact = components["schemas"]["Contact"];

/**
 * What a contact's page shows while its 360 read is still in flight.
 *
 * Once the page's own contact read succeeds, its name and title are shown.
 * The title keeps its provider mark. No action, tab or count is mounted here,
 * because the base read does not authorize any of them.
 *
 * Before that read answers, the name comes from the list the open was clicked
 * from, if one is cached. A direct link, a reload or a mail link has no list,
 * so the heading stays empty until the record answers.
 *
 * The page sets hideIdentity when the 360 read has already failed, so a refused
 * record never shows a name while the base read is still settling.
 */
export function ContactOpening({
  id,
  contact,
  hideIdentity = false,
}: Readonly<{ id: string; contact?: Contact; hideIdentity?: boolean }>) {
  const t = useT();
  const confirmed = contact?.id === id ? contact : undefined;
  return (
    <div className="wrap">
      {!hideIdentity &&
        (confirmed ? (
          <>
            {confirmed.full_name.trim() && (
              <Heading size="xlarge">{confirmed.full_name}</Heading>
            )}
            {confirmed.title && (
              <div>
                <BoughtText
                  value={confirmed.title}
                  bought={boughtFields(confirmed).get("title")}
                />
              </div>
            )}
          </>
        ) : (
          <CachedContactHeading id={id} />
        ))}
      <PendingBody label={t("contact.page.loading")} />
    </div>
  );
}

// List names are only a fallback until the record answers, never a source of fields.
function CachedContactHeading({ id }: Readonly<{ id: string }>) {
  const name = useCachedRecordName("contacts", id);
  return name ? <Heading size="xlarge">{name}</Heading> : null;
}
