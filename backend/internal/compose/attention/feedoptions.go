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

// WithDealStandings binds the reader that puts a deal's already-written standing
// on its queue row. An option for the reason WithDealMoves is one.
//
// Unbound, a deal row still says what to do and still carries its typed reasons;
// what it loses is the verdict word above them, and the night's brief finding
// still reaches a row this feed surfaced. dealstanding.go states why the floor
// is no verdict rather than one this pass computes.
func (s *Service) WithDealStandings(d DealStandings) *Service {
	s.dealStandings = d
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

// WithPromiseLoad binds the board's counting reader for commitments due.
//
// Unbound, the board draws no promises column — and here the zero would be
// worse than elsewhere: an installation that extracts no claims would report a
// team that promised nothing, when the truth is that nobody was listening.
func (s *Service) WithPromiseLoad(p PromiseLoad) *Service {
	s.promiseLoad = p
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

// WithWalks binds the store that holds one reader's walk still while they page
// it — an option for the reason WithPins is one.
//
// UNBOUND MEANS THE OLD PAGING, exactly: an offset into a ranking rebuilt on
// every read, with the cost cursor.go states. That is a real answer rather than
// a degraded one — an installation that wires no snapshot store pages the way
// this queue always did — and it is also the rollout lever, since a nil seam
// leaves every existing walk behaving as it did before this shipped.
func (s *Service) WithWalks(w Walks) *Service {
	s.walks = w
	return s
}
