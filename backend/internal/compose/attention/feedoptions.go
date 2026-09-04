// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The OPTIONAL readers, bound one at a time.
//
// Options rather than more positional arguments: NewService already takes
// nineteen, and the next reader to add one would be adding the twentieth to a
// call nobody can read.
//
// Each is absent when it is not bound, and absent is not empty — "this feed does
// not do commitments" is a different fact from "you owe nobody anything". The
// two that break that rule say so in their own comment, because for them an
// unanswerable question is a refusal rather than a missing lane.

// WithWaiting binds the who-is-waiting reader.
//
// An option rather than another positional argument: NewService already takes
// nineteen, and the next reader to add one would be adding the twentieth to a
// call nobody can read. The lane is absent when it is not bound, which is the
// same promise every optional lane makes.
func (s *Service) WithWaiting(w Waiting) *Service {
	s.waiting = w
	return s
}

// WithUndelivered binds the given-up-on-sends reader — an option for the reason
// WithWaiting is one, which this lane would otherwise have been the first to
// disprove.
func (s *Service) WithUndelivered(u Undelivered) *Service {
	s.undelivered = u
	return s
}

// WithIntroductions binds the reader for asks waiting on this colleague — an
// option for the reason WithWaiting is one.
//
// Unbound, the lane is ABSENT rather than empty: "this installation does not do
// introductions" is a different fact from "nobody has asked you for one", and a
// colleague told the second when the first is true would stop looking.
func (s *Service) WithIntroductions(i Introductions) *Service {
	s.introductions = i
	return s
}

// WithMachineSender binds the rule that tells a sending system from a person.
func (s *Service) WithMachineSender(is MachineSender) *Service {
	s.machine = is
	return s
}

// WithDealFacts binds the reader that puts a deal's own figures on a row whose
// producer carried only its id. An option for the reason WithWaiting is one.
//
// Unbound, those rows travel with a name and no figures, which is what they did
// before this seam existed — a smaller card, never a wrong one.
func (s *Service) WithDealFacts(f DealFacts) *Service {
	s.dealFacts = f
	return s
}

// WithDealMoves binds the reader that puts a deal's already-decided next step
// on its queue row. An option for the reason WithDealFacts is one.
//
// Unbound, a deal row names its problem and no step — what every deal row did
// before this seam, and never a wrong step.
func (s *Service) WithDealMoves(m DealMoves) *Service {
	s.dealMoves = m
	return s
}

// WithTeammates binds the membership question a team-scoped reader's named-owner
// ask is decided by. An option for the reason WithWaiting is one.
//
// Unbound, a team-scoped reader naming somebody else is REFUSED — the opposite
// of how the optional lanes above degrade, and deliberately so: those answer
// "this installation has no such lane", while this one answers "may I", and an
// unanswerable may-I is a no.
func (s *Service) WithTeammates(t Teammates) *Service {
	s.teammates = t
	return s
}

// WithLeadResponses binds the inbound leads still owed a first reply.
//
// Read BESIDE the assembled day rather than as a fifteenth lane, the way the
// waiting-customer source already is: /attention publishes a fourteen-lane
// promise, and this is not one of them. The queue is where the two orders meet.
func (s *Service) WithLeadResponses(l LeadResponses) *Service {
	s.leads = l
	return s
}

// WithOverdueLoad binds the team board's counting reader for tasks — an option
// for the reason WithWaiting is one.
//
// Unbound, the board draws no overdue column rather than one of zeros. A column
// of zeros would tell a lead their team is up to date, which is the answer this
// surface exists to stop getting wrong.
func (s *Service) WithOverdueLoad(o OverdueLoad) *Service {
	s.overdueLoad = o
	return s
}

// WithPins binds the reader's own pinned rows — an option for the reason
// WithWaiting is one.
//
// Unbound means no pins, and a day assembled that way ranks exactly as it did
// before pinning existed. That is the honest absence rather than a degraded
// answer: an installation with no pin store has none.
func (s *Service) WithPins(p Pins) *Service {
	s.pins = p
	return s
}
