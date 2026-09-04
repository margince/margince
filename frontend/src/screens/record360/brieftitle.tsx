// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of the card a machine wrote: the mark that says WHO produced this
// reading, and WHAT it is a reading of.
//
// One line (DESIGN.md §7): the mark leads it, so a reader has the authorship
// before they weigh a word of it, and the name follows as the subject.
//
// The mark carries the claim ALONE. Indigo on its own tile means "a machine
// wrote what is under this" everywhere in the product, so the sentence that
// used to close the line ("Margince read this record") said a second time, in
// the eyebrow's shout, what the tile beside it had already said — and it read
// as a caption somebody had typed onto the record's name.
//
// In the kit rather than on the account page, because every record that gets
// a written reading owes the same claim in the same shape. Two records
// spelling it themselves is how one of them comes to say it differently.

import { Sparkles } from "lucide-react";
import { useT } from "../../i18n";
import "../company360.css";

/** BriefTitle is the head's one line, over whichever record it reads. */
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
      <span className="co-360-subject">
        {name ? t("co.360.subject", { name }) : t("co.360.subjectUnnamed")}
      </span>
    </span>
  );
}
