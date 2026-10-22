import { type MouseEvent, type PointerEvent, useRef } from "react";

type BackdropHandlers = Readonly<{
  onPointerDown: (event: PointerEvent<HTMLElement>) => void;
  onPointerUp: (event: PointerEvent<HTMLElement>) => void;
  onClick: (event: MouseEvent<HTMLElement>) => void;
}>;

/**
 * The handlers that let a click on a dialog's overlay dismiss it. A browser
 * sends the click of a drag to the element the press and the release share, so
 * a text selection dragged from the dialog out to the backdrop arrives as a
 * click on the overlay. Only a press and a release both on the overlay count.
 */
export function useBackdropDismiss(
  onDismiss: () => void,
  leaving: boolean,
): BackdropHandlers {
  const pressedOnBackdrop = useRef(false);
  return {
    onPointerDown: (event) => {
      pressedOnBackdrop.current = event.target === event.currentTarget;
    },
    onPointerUp: (event) => {
      pressedOnBackdrop.current &&= event.target === event.currentTarget;
    },
    onClick: (event) => {
      const onBackdrop =
        pressedOnBackdrop.current && event.target === event.currentTarget;
      pressedOnBackdrop.current = false;
      if (onBackdrop && !leaving) {
        onDismiss();
      }
    },
  };
}
