// What the render gate (fe-uat.mjs) reads off Storybook's own channel to decide
// that a story finished rendering, and whether its render or play() failed.
import {
  PLAY_FUNCTION_THREW_EXCEPTION,
  STORY_ERRORED,
  STORY_FINISHED,
  STORY_MISSING,
  STORY_RENDERED,
  STORY_THREW_EXCEPTION,
  UNHANDLED_ERRORS_WHILE_PLAYING,
} from "storybook/internal/core-events";

// `storyRendered` is the success path; a throwing play() never reaches it, and a
// module that throws at import emits only `storyMissing`.
export const SETTLED_EVENTS = [
  STORY_RENDERED,
  STORY_FINISHED,
  STORY_THREW_EXCEPTION,
  STORY_ERRORED,
  PLAY_FUNCTION_THREW_EXCEPTION,
  STORY_MISSING,
];

// Not storyFinished's status: it folds in the a11y addon's axe report, which
// this gate does not enforce.
export const FAILURE_EVENTS = [
  PLAY_FUNCTION_THREW_EXCEPTION,
  STORY_THREW_EXCEPTION,
  STORY_ERRORED,
  UNHANDLED_ERRORS_WHILE_PLAYING,
  STORY_MISSING,
];

const firstLine = (text) => String(text ?? "").split("\n")[0];

// outcomeErrors turns the last arguments of each FAILURE_EVENTS entry (null
// when the event never fired) into the gate's error lines.
export function outcomeErrors(outcome) {
  const errors = [];
  const play = outcome[PLAY_FUNCTION_THREW_EXCEPTION]?.[0];
  const threw = outcome[STORY_THREW_EXCEPTION]?.[0];
  if (play)
    errors.push(`play() threw ${play.name}: ${firstLine(play.message)}`);
  // A throwing play() is re-reported as a story exception with the same error.
  if (threw && threw.message !== play?.message) {
    errors.push(`the story threw ${threw.name}: ${firstLine(threw.message)}`);
  }
  const errored = outcome[STORY_ERRORED]?.[0];
  if (errored) {
    errors.push(
      `the story errored: ${errored.title} — ${firstLine(errored.description)}`,
    );
  }
  for (const unhandled of outcome[UNHANDLED_ERRORS_WHILE_PLAYING]?.[0] ?? []) {
    errors.push(
      `unhandled error while playing: ${firstLine(unhandled.message)}`,
    );
  }
  // Fired with no argument when nothing was selected, so presence is the signal.
  const missing = outcome[STORY_MISSING];
  if (missing) {
    const which = missing[0] === undefined ? "" : ` (${String(missing[0])})`;
    errors.push(
      `Storybook could not load the story${which}: its module failed to import or no longer exports it`,
    );
  }
  return errors;
}
