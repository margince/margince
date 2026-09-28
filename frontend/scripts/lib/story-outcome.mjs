// What the render gate (fe-uat.mjs) reads off Storybook's own channel to decide
// that a story finished rendering, and whether its render or play() failed.

// Any one of these ends a story's render. `storyRendered` is the success path;
// a play() that throws never reaches it and ends in the others.
export const SETTLED_EVENTS = [
  "storyRendered",
  "storyFinished",
  "storyThrewException",
  "storyErrored",
  "playFunctionThrewException",
];

// Not storyFinished's status: it folds in the a11y addon's axe report, which
// this gate does not enforce.
export const FAILURE_EVENTS = [
  "playFunctionThrewException",
  "storyThrewException",
  "storyErrored",
  "unhandledErrorsWhilePlaying",
];

const firstLine = (text) => String(text ?? "").split("\n")[0];

// outcomeErrors turns the last payload of each FAILURE_EVENTS entry (null when
// the event never fired) into the gate's error lines.
export function outcomeErrors(outcome) {
  const errors = [];
  const play = outcome.playFunctionThrewException;
  const threw = outcome.storyThrewException;
  if (play)
    errors.push(`play() threw ${play.name}: ${firstLine(play.message)}`);
  // A throwing play() is re-reported as a story exception with the same error.
  if (threw && threw.message !== play?.message) {
    errors.push(`the story threw ${threw.name}: ${firstLine(threw.message)}`);
  }
  const errored = outcome.storyErrored;
  if (errored) {
    errors.push(
      `the story errored: ${errored.title} — ${firstLine(errored.description)}`,
    );
  }
  for (const unhandled of outcome.unhandledErrorsWhilePlaying ?? []) {
    errors.push(
      `unhandled error while playing: ${firstLine(unhandled.message)}`,
    );
  }
  return errors;
}
