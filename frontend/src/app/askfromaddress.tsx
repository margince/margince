import { lazy, Suspense, useEffect, useState } from "react";
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
  const open = dials.get(ASK_PARAM) !== undefined;
  // Mounted for the rest of the session once it has been opened, so closing is
  // `open={false}` and not an unmount. `Modal` animates its exit through
  // `usePresence`, which needs the element to outlive the flag by the length of
  // the transition; pulled out from under it, the panel vanished in one frame.
  // The dialog's own `!open` effect runs on the same edge and is what lets a
  // reopen with the SAME carried question fill the box again.
  const [everOpened, setEverOpened] = useState(false);
  useEffect(() => {
    if (open) {
      setEverOpened(true);
    }
  }, [open]);
  if (!(open || everOpened)) {
    return null;
  }
  return (
    <Suspense fallback={null}>
      <AskMarginceModal
        open={open}
        carriedQuestion={dials.get(ASK_QUESTION_PARAM)}
        onClose={closeAsk}
      />
    </Suspense>
  );
}
