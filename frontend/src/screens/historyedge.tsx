import { Link2 } from "lucide-react";
import type { components } from "../api/schema";
import { Chip } from "../design-system/readings";
import { useT } from "../i18n";
import { EntityRef } from "./entityref";

// What an edge row shows instead of a field diff.
//
// A link is recorded against the LINK, so `role`, `started_at` and the
// primary-employer flag in the images belong to it rather than to either record
// it joins. Drawn as this record's fields they invent fields it does not have,
// and the label map has no word for any of them — so the row keeps the sentence
// the read already wrote in prose and adds only the two things the sentence
// cannot be: a mark saying this was a link and not an edit, and a way to reach
// the record at the other end.

type HistoryEdge = NonNullable<components["schemas"]["HistoryEdge"]>;

export function HistoryEdgeDetail({ edge }: Readonly<{ edge: HistoryEdge }>) {
  const t = useT();
  return (
    <span className="entry-edge">
      {/* A Chip and not a Badge: which kind of change this row is, is a fact
          about the row rather than a status it might leave. */}
      <Chip icon={Link2}>{t("history.edge.marker")}</Chip>
      {/* A null label is the read saying it could not name the other end — so
          the reference is handed over unnamed and EntityRef's own reading
          decides what to say, which keeps one spelling of "no name for this
          record" on a page that already draws several. */}
      <EntityRef
        kind={edge.other_entity_type}
        id={edge.other_entity_id}
        name={edge.other_label}
      />
    </span>
  );
}
