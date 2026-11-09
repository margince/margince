import type { components } from "../../api/schema";

type Review = components["schemas"]["SdrTeamReview"];
type Withheld = Review["withheld_figures"][number];

/** Whether the server left a figure out because this reader may not see it. */
export function figureWithheld(review: Review, figure: Withheld): boolean {
  return review.withheld_figures.includes(figure);
}
