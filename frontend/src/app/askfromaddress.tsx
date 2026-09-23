import { lazy, Suspense } from "react";
import {
  ASK_PARAM,
  ASK_QUESTION_PARAM,
  closeAsk,
  useUrlParams,
} from "./urlstate";

// The Ask dialog, opened by the ADDRESS rather than by a screen.
//
// A question occurs in the middle of other work — a reader is on a deal,
// something comes up, and the answer sends them back to the deal — so it stands
// over the page they were on instead of taking them somewhere. The dial is what
// carries it: a reload keeps the question, and the link can be sent to somebody
// else.
//
// Its own file because the shell should not have to know how asking works to
// render it — App carries the routes, the rail and the palette, and a dialog
// mounted beside them is one more thing it would otherwise hold the wiring for.
//
// Lazy, because the dialog brings the markdown renderer with it and most
// sessions never open one.
const AskMarginceModal = lazy(() =>
  import("../screens/corpusask").then((m) => ({ default: m.AskMarginceModal })),
);

export function AskFromAddress() {
  const [dials] = useUrlParams();
  // Presence opens it; the question rides beside it. Two dials because an empty
  // one does not survive the address — see urlstate.
  if (dials.get(ASK_PARAM) === undefined) {
    return null;
  }
  return (
    <Suspense fallback={null}>
      <AskMarginceModal
        open
        carriedQuestion={dials.get(ASK_QUESTION_PARAM)}
        onClose={closeAsk}
      />
    </Suspense>
  );
}
