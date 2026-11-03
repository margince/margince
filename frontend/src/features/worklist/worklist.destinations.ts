import type { components } from "../../api/schema";
import { viewOfCategory, type WorklistView } from "../../lib/worklistviews";

type WorklistItem = components["schemas"]["WorklistItem"];

// Which view actually CONTAINS a folded group's members, read from the one
// category map. Sending every group to Decisions would filter a system
// group's own failures out of view, and the Review verb would open an empty page.
export function reviewView(item: WorklistItem): WorklistView {
  return viewOfCategory(item.category);
}
