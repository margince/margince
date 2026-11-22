// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ActionBringInManager asks the deal's team manager to look at the deal. Its
// arguments are a task for that manager, which the client files on accept.
const ActionBringInManager = "bring_in_manager"

const (
	reasonBigDeal      = "big_deal"
	reasonSingleThread = "single_contact"
	reasonStuck        = "stuck"
	// taskTypeReview is the task type that asks somebody to check a record.
	taskTypeReview = "review"
)

// ManagerStanding is what the Worklist needs about a deal beyond its own
// rows: who leads the owner's team, and where the deal stands in the team's
// latest forecast freeze.
type ManagerStanding struct {
	// Managers are the owner's team leads. Empty means nobody to bring in.
	Managers    []ids.UUID
	BaseMinor   int64
	AboveMedian bool
	Stuck       bool
	StuckStage  string
}

// ManagerStandings reads the standing of each deal, keyed by deal id, given the
// owner of each deal.
type ManagerStandings func(ctx context.Context, tx pgx.Tx, owners map[ids.UUID]ids.UUID) (map[ids.UUID]ManagerStanding, error)

// WithManagerStandings binds the read that lets a Worklist row offer "bring in
// the manager". Without it no row offers it.
func (s *Service) WithManagerStandings(read ManagerStandings) *Service {
	s.managerStandings = read
	return s
}

var managerWords = struct {
	Offer, Subject, BigDeal, SingleContact, Stuck langcopy.Phrase
}{
	Offer:         langcopy.Phrase{En: "Bring in your manager: %s.", De: "Holen Sie Ihre Führungskraft dazu: %s.", Vi: "Hãy mời quản lý của bạn tham gia: %s."},
	Subject:       langcopy.Phrase{En: "Look at %s with me", De: "Schauen Sie sich %s mit mir an", Vi: "Cùng xem %s với tôi"},
	BigDeal:       langcopy.Phrase{En: "its value is above the team's median", De: "der Wert liegt über dem Median des Teams", Vi: "giá trị cao hơn mức trung vị của nhóm"},
	SingleContact: langcopy.Phrase{En: "only one contact is engaged", De: "nur ein Kontakt ist eingebunden", Vi: "chỉ có một liên hệ đang tham gia"},
	Stuck:         langcopy.Phrase{En: "it is stuck in its stage", De: "er steckt in seiner Phase fest", Vi: "giao dịch đang bị kẹt ở giai đoạn hiện tại"},
}

type managerReason struct {
	code, fact string
	phrase     langcopy.Phrase
}

// offersManager is true for a deal whose own move is a generic next step.
// Answering a waiting buyer, preparing a meeting or finishing a task stays
// first, because somebody is waiting on the rep.
func offersManager(move crmcontracts.DealStatusCardMove) bool {
	if move.Recommendation == nil {
		return false
	}
	switch moveRule(move.Recommendation.Rule) {
	case ruleNextStep, ruleFirstOutreach, ruleMeetingRequest:
		return true
	}
	return false
}

// singleThreaded applies the rule the account coverage view and the deal page
// share, so every surface calls the same deals single-threaded.
func singleThreaded(f facts, maxSeats int) (string, bool) {
	var engaged []string
	for _, seat := range f.seats {
		if seat.Engaged {
			engaged = append(engaged, seat.ContactID.String())
		}
	}
	slices.Sort(engaged)
	return strings.Join(engaged, ","), reporting.SingleThreaded(f.lastExchange != nil, len(engaged), maxSeats)
}

func managerReasons(f facts, standing ManagerStanding, maxSeats int) []managerReason {
	var out []managerReason
	if standing.AboveMedian {
		out = append(out, managerReason{reasonBigDeal, fmt.Sprint(standing.BaseMinor), managerWords.BigDeal})
	}
	if fact, single := singleThreaded(f, maxSeats); single {
		out = append(out, managerReason{reasonSingleThread, fact, managerWords.SingleContact})
	}
	if standing.Stuck {
		out = append(out, managerReason{reasonStuck, standing.StuckStage, managerWords.Stuck})
	}
	return out
}

// withManagerMoves swaps a generic next step for "bring in the manager" where
// the deal qualifies and its owner has a team manager. The move's fingerprint
// holds each reason and the fact behind it, so a set-aside lifts when a fact
// changes: the amount, the engaged contacts, or the stage a deal sticks in.
func (s *Service) withManagerMoves(
	ctx context.Context, tx pgx.Tx, current map[ids.UUID]facts, moves map[ids.UUID]crmcontracts.DealStatusCardMove,
) error {
	if s.managerStandings == nil {
		return nil
	}
	owners := map[ids.UUID]ids.UUID{}
	for id, move := range moves {
		if owner := current[id].deal.OwnerId; owner != nil && offersManager(move) {
			owners[id] = ids.UUID(*owner)
		}
	}
	if len(owners) == 0 {
		return nil
	}
	standings, err := s.managerStandings(ctx, tx, owners)
	if err != nil {
		return err
	}
	thresholds, _, err := reporting.AccountCoverageThresholds(ctx, tx)
	if err != nil {
		return err
	}
	for id := range owners {
		standing := standings[id]
		reasons := managerReasons(current[id], standing, *thresholds.MaxEngagedSeats)
		if len(reasons) == 0 || len(standing.Managers) == 0 {
			continue
		}
		offer, err := managerMove(current[id], standing.Managers[0], reasons)
		if err != nil {
			return err
		}
		moves[id] = offer
	}
	return nil
}

func managerMove(f facts, manager ids.UUID, reasons []managerReason) (crmcontracts.DealStatusCardMove, error) {
	said := statusCopyFor(cmp.Or(f.say, f.lang))
	words := make([]string, len(reasons))
	marks := make([]owedwork.Mark, len(reasons))
	for i, reason := range reasons {
		words[i] = said.Say(reason.phrase)
		marks[i] = owedwork.Mark{Kind: "manager_reason_" + reason.code, ID: reason.fact}
	}
	why := strings.Join(words, "; ")
	subject := fmt.Sprintf(said.Say(managerWords.Subject), f.deal.Name)
	offer := ruleMove(ruleBringInManager, fmt.Sprintf(said.Say(managerWords.Offer), why),
		map[string]any{
			"subject":     subject,
			"body":        capitalized(why) + ".",
			"assignee_id": manager,
			"task_type":   taskTypeReview,
			argLinks:      []map[string]any{{argEntityType: linkDeal, argEntityID: f.deal.Id}},
			"source":      provenance.RecordSourceManual,
		})
	offer, err := assessMove(f, offer)
	if err != nil {
		return crmcontracts.DealStatusCardMove{}, err
	}
	return stamped(offer, marks...), nil
}

// capitalized upper-cases the first letter, which the joined reasons start
// without so that they read mid-sentence in the move's reason.
func capitalized(text string) string {
	first, size := utf8.DecodeRuneInString(text)
	return string(unicode.ToUpper(first)) + text[size:]
}
