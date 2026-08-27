// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of the card a machine wrote: WHO produced this reading, then WHAT
// it is a reading of.
//
// Two lines, in that order, because they answer different questions and the
// first is the one a reader needs before they weigh a word of it — a machine
// read this record, and here is the record it read. The eyebrow carries the
// authorship and the name carries the subject; folded into one line, the
// authorship reads as part of the record's name.
//
// In the kit rather than on the account page, because every record that gets
// a written reading owes the same claim in the same words. Two records
// spelling it themselves is how one of them comes to say it differently.

import { Sparkles } from "lucide-react";
import { Eyebrow } from "../../design-system/eyebrow";
import { useT } from "../../i18n";
import "../company360.css";

/** BriefTitle is the head band's two lines, over whichever record it reads. */
export function BriefTitle({ name }: Readonly<{ name?: string }>) {
  const t = useT();
  return (
    <span className="co-360-title">
      {/* The mark that says a machine produced what follows, on the one band
          that claims it. Indigo everywhere it appears, because that is what
          the hue means here — authorship, never status. On a tile of its own
          rather than inline with the words: this claim is the reason the card
          under it exists, and a 12px glyph inside an eyebrow said the
          opposite. */}
      <span className="co-360-mark" aria-hidden="true">
        <Sparkles />
      </span>
      <Eyebrow as="span">{t("co.360.title")}</Eyebrow>
      <span className="co-360-subject">
        {name ? t("co.360.subject", { name }) : t("co.360.subjectUnnamed")}
      </span>
    </span>
  );
}
