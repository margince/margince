import type { ReactNode } from "react";
import { Badge } from "../../design-system/atoms";
import { IdentityLine } from "../../design-system/identityline";
import { useTooltip } from "../../design-system/tooltip";
import { useLocale, useT } from "../../i18n";
import { type ContactTab, contactTabRoute } from "../../lib/contacttab";
import { revealOnceMounted } from "../../lib/reveal";
import { RecordAccess } from "../records/recordaccess";
import { navigateKeepingReturn } from "../worklist/worklist.return";
import { BRIEF_ANCHOR } from "./contactoverview";
import type { Contact360 } from "./contactpage";
import { contactStanding, standingSentences } from "./contactrail";

// The marks under the name: the relationship's standing and who may read the
// record. Standing is one badge for verdict and trend, absent when the touch
// dates are withheld from this reader. A rep glances for both first, and both
// are pills, so they share a line.
export function ContactMarks({
  view,
  tab,
}: Readonly<{ view: Contact360; tab: ContactTab }>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const standing = contactStanding(view, t);
  // The brief lives on the overview: from any other tab the press goes there
  // first, and the reveal waits for the anchor to be drawn.
  const showBrief = () => {
    if (tab !== "overview") {
      navigateKeepingReturn(contactTabRoute(view.contact.id, "overview"));
    }
    revealOnceMounted(BRIEF_ANCHOR);
  };
  return (
    <IdentityLine separator="space">
      {standing && (
        <StandingChip
          text={standingSentences(view, t, locale)}
          onPress={showBrief}
        >
          <Badge tone={standing.tone}>{standing.words}</Badge>
        </StandingChip>
      )}
      <RecordAccess
        key={view.contact.id}
        kind="contact"
        record={view.contact}
      />
    </IdentityLine>
  );
}

// The standing chip: a tooltip says why it reads as it does. Pressing it takes
// the reader to the brief, which lays out the readings behind the word in full.
// A button and not a link, because the brief is on this page.
function StandingChip({
  text,
  onPress,
  children,
}: Readonly<{ text: string; onPress: () => void; children: ReactNode }>) {
  const { ref, trigger, tip } = useTooltip<HTMLButtonElement>(text);
  return (
    <button
      type="button"
      className="pe-standing-chip"
      ref={ref}
      {...trigger}
      onClick={onPress}
    >
      {children}
      {tip}
    </button>
  );
}
