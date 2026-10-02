import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { useCachedRecordName } from "./recordidentity";

/**
 * What a contact's page shows while its own read is still in flight.
 *
 * The identity comes from the list the open was clicked from rather than from
 * the read, so the head is on screen before the network answers — which is what
 * makes an open feel instant, and is the one such claim that does not depend on
 * how fast the machine is.
 *
 * The seed is looked up HERE rather than by the page, so the cache is read only
 * while a contact is opening: a page that has its record does not re-scan every
 * cached list on every render to seed a heading it is no longer showing.
 *
 * Contact-specific, holding its own list key and label, because one record page
 * wants this today. The generic version can be lifted out when a second one
 * does, and will then have two call sites to tell it what varies.
 *
 * There is no name when the open had no list behind it: a pasted address, a
 * reload, a link from mail. Then this is the placeholder it always was, because
 * there is no identity to show and nothing honest to invent.
 */
export function ContactOpening({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const name = useCachedRecordName("contacts", id);
  return (
    <div className="wrap">
      {name ? <Heading size="xlarge">{name}</Heading> : null}
      {t("contact.page.loading")}
    </div>
  );
}
