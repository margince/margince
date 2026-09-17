import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { LinkedCase } from "./privacy.caselink";

/**
 * What the queue says when it cannot show the case it was sent to open.
 *
 * Drawn ABOVE the empty state as well as above the list. A reader who narrows
 * the facet after following a link can empty the page while the address still
 * names a case, and "nothing here" alone answers a question they did not ask —
 * they came to read one case, not to learn whether this filter has rows.
 */
export function LinkedCaseNotice({ linked }: Readonly<{ linked: LinkedCase }>) {
  const t = useT();
  if (linked.kind !== "absent") {
    return null;
  }
  return (
    <Callout tone="warning" title={t("privacy.caseNotHere")}>
      {t("privacy.caseNotHereBody")}
    </Callout>
  );
}
