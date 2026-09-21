import { trimSubjectSpace } from "./servertrim";

// Re: is normalized; forward markers remain part of the selected subject.
//
// The loop is ReplySubject's own (activities/replysubject.go) rather than one
// regex over the lot: strip a single "re:" and trim what it left behind, so
// "Re: RE:  x" collapses to one prefix the same way on both sides of the wire.
// The shared corpus (activities/testdata/replysubjects.txt) is what holds them
// together, and it includes Unicode whitespace — NEXT LINE included.
export function replySubject(subject: string): string {
  let topic = trimSubjectSpace(subject);
  while (topic.slice(0, 3).toLowerCase() === "re:") {
    topic = trimSubjectSpace(topic.slice(3));
  }
  return `Re: ${topic}`;
}
