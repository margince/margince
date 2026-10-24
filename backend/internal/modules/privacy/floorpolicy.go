// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Which correspondence floor binds this installation, resolved once per
// destructive decision and handed to every statement that decides.
//
// The installation selects one jurisdiction. Unset keeps the strictest floor
// of every compiled-in pack, because a zero floor shields nothing. A new
// release must not expose mail the floor kept the day before. A change to a
// weaker floor carries the old one for mail already held.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// CountryReader answers the installation's selected jurisdiction inside the
// caller's transaction, or "" when none is set. compose injects identity's,
// since privacy cannot import identity. A failed read is an error, never "no
// pack": a policy built from a failed read could be weaker than the law.
type CountryReader func(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error)

// WithCountry returns an eraser that reads the selected jurisdiction for the
// correspondence floor. Without it the floor is the strictest compiled-in one.
func (e *Eraser) WithCountry(country CountryReader) *Eraser {
	clone := *e
	clone.country = country
	return &clone
}

// WithExtensions gives the retention service's eraser the extension tables to
// clear.
func (s *RetentionService) WithExtensions(r extprivacy.Registry) *RetentionService {
	out := *s
	out.eraser = s.eraser.WithExtensions(r)
	return &out
}

// WithCountry is the retention service's spelling: its selectors, and the
// eraser its actions run through, read the same jurisdiction.
func (s *RetentionService) WithCountry(country CountryReader) *RetentionService {
	out := *s
	out.eraser = s.eraser.WithCountry(country)
	return &out
}

// errKeptByFloor reports that a row selected for destruction is shielded by
// the floor in force now. A destructive executor returns it and its caller
// keeps the row.
var errKeptByFloor = errors.New("privacy: the statutory floor in force now keeps this record")

// refuseIfShieldedNow re-asks the floor for one activity inside the
// destroying transaction. Selection and destruction run in different
// transactions, so a jurisdiction change between them could otherwise destroy
// mail the floor now in force protects.
func (s *RetentionService) refuseIfShieldedNow(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	floor, err := ResolveFloorPolicy(ctx, tx, s.eraser.country)
	if err != nil {
		return err
	}
	floorCurrent, floorCarried := floor.sqlArgs()
	var shielded bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $1 AND `+handelsbriefShielded(2, 3)+`)`,
		id, floorCurrent, floorCarried).Scan(&shielded); err != nil {
		return fmt.Errorf("privacy: re-checking the statutory floor before destroying a record: %w", err)
	}
	if shielded {
		return errKeptByFloor
	}
	return nil
}

// erasableUnderFloor refuses a contact the erasure may not touch, then
// resolves the floor the cascade applies. Erasing the contact a Handelsbrief
// hangs off must not destroy the correspondence below its floor.
func (e *Eraser) erasableUnderFloor(ctx context.Context, tx pgx.Tx, subject ids.ContactID) (FloorPolicy, error) {
	if err := requireErasableContact(ctx, tx, subject); err != nil {
		return FloorPolicy{}, err
	}
	// Asked up front, so a refusal arrives before the cascade changes anything.
	// clearExtensionRows asks again for the leads and activities it finds later.
	if err := refuseHeldExtensionRows(ctx, tx, e.extensions, map[extdecl.RecordKind][]ids.UUID{extdecl.Contact: {subject.UUID}}); err != nil {
		return FloorPolicy{}, err
	}
	return ResolveFloorPolicy(ctx, tx, e.country)
}

// CarriedFloor is a floor still binding the mail that occurred before Before.
type CarriedFloor struct {
	Class  jurisdiction.RetentionClass
	Before time.Time
}

// FloorPolicy is the evaluated correspondence floor: what binds new mail, and
// what still binds older mail after a change to a weaker one.
type FloorPolicy struct {
	// Selected is the installation's jurisdiction, empty when none is set.
	Selected jurisdiction.Code
	// Current binds every row. Its zero value shields nothing.
	Current jurisdiction.RetentionClass
	// Carried also binds rows that occurred before its Before. Nil when no
	// change ever weakened the floor.
	Carried *CarriedFloor
}

// FloorCarry is the stored form of a carried floor. Its zero value is "none".
type FloorCarry struct {
	Class  jurisdiction.RetentionClassName `json:"class,omitempty"`
	Years  int                             `json:"years,omitempty"`
	Months int                             `json:"months,omitempty"`
	Days   int                             `json:"days,omitempty"`
	Anchor jurisdiction.Anchor             `json:"anchor,omitempty"`
	Before *time.Time                      `json:"before,omitempty"`
}

// carried reads the stored value back as a floor, or nil for "none".
func (c FloorCarry) carried() *CarriedFloor {
	if c.Before == nil {
		return nil
	}
	return &CarriedFloor{
		Class:  jurisdiction.RetentionClass{Name: c.Class, Keep: jurisdiction.Period{Years: c.Years, Months: c.Months, Days: c.Days}, Anchor: c.Anchor},
		Before: *c.Before,
	}
}

func carryOf(f CarriedFloor) FloorCarry {
	before := f.Before
	return FloorCarry{
		Class: f.Class.Name, Years: f.Class.Keep.Years, Months: f.Class.Keep.Months, Days: f.Class.Keep.Days,
		Anchor: f.Class.Anchor, Before: &before,
	}
}

// RetentionFloorCarry is the carried floor. Never lowered: a later weakening
// keeps the stricter class and moves Before forward. It survives a data reset,
// like the country it follows, because the mail it protects can predate the
// reset's own records. It stays a settings.Define entry, out of the boot
// sources: a jurisdiction change writes it, and no operator sets it.
var RetentionFloorCarry = settings.Define[FloorCarry](
	"privacy.retention_floor_carry",
	retentionPolicyObject,
	settingsAuditVerb,
	FloorCarry{},
	func(c FloorCarry) error {
		if c.Before == nil {
			if c != (FloorCarry{}) {
				return errors.New("a carried floor without a change time names no rows it binds")
			}
			return nil
		}
		floor := c.carried()
		if err := floor.Class.Name.Validate(); err != nil {
			return err
		}
		if err := floor.Class.Keep.Validate(); err != nil {
			return err
		}
		return floor.Class.Anchor.Validate()
	},
).AsInstallationIdentity().MachineryApplied()

// ResolveFloorPolicy evaluates the floor for one decision, inside the
// caller's transaction.
//
// A nil reader reads as an unset jurisdiction, the strictest compiled-in
// floor, which can only shield more than the selected pack would.
func ResolveFloorPolicy(ctx context.Context, tx pgx.Tx, country CountryReader) (FloorPolicy, error) {
	return resolveFloorPolicyAt(ctx, tx, country, time.Now())
}

// resolveFloorPolicyAt orders the compiled-in floors at ref, so every policy
// in one retention pass compares mixed-unit periods at the same instant.
func resolveFloorPolicyAt(ctx context.Context, tx pgx.Tx, country CountryReader, ref time.Time) (FloorPolicy, error) {
	var code jurisdiction.Code
	if country != nil {
		var err error
		if code, err = country(ctx, tx); err != nil {
			return FloorPolicy{}, fmt.Errorf("privacy: reading the installation's jurisdiction for the retention floor: %w", err)
		}
	}
	carry, err := settings.ApplyTx(ctx, tx, RetentionFloorCarry)
	if err != nil {
		return FloorPolicy{}, fmt.Errorf("privacy: reading the carried retention floor: %w", err)
	}
	current, err := floorFor(code, ref)
	if err != nil {
		return FloorPolicy{}, err
	}
	return FloorPolicy{Selected: code, Current: current, Carried: carry.carried()}, nil
}

// floorFor is the correspondence floor a jurisdiction setting puts in force:
// the selected pack's, or the strictest compiled-in one when none is selected.
func floorFor(code jurisdiction.Code, ref time.Time) (jurisdiction.RetentionClass, error) {
	if code == "" {
		return statutoryCorrespondenceFloor(ref), nil
	}
	pack, ok := jurisdiction.For(code)
	if !ok {
		return jurisdiction.RetentionClass{}, fmt.Errorf("privacy: the installation's jurisdiction %q has no compiled-in pack, so no floor can be read for it", code)
	}
	return correspondenceClassOf(pack), nil
}

// CorrespondenceFloorOf is the floor selecting code would put in force, for a
// screen that says what a change would do before it is made.
func CorrespondenceFloorOf(code jurisdiction.Code) (jurisdiction.RetentionClass, error) {
	return floorFor(code, time.Now())
}

// correspondenceClassOf is a pack's commercial-correspondence class, or the
// zero class when it declares none.
func correspondenceClassOf(pack jurisdiction.Pack) jurisdiction.RetentionClass {
	return classOf(pack, jurisdiction.CommercialCorrespondence)
}

// classOf is a pack's class of that name, or the zero class when it declares none.
func classOf(pack jurisdiction.Pack, name jurisdiction.RetentionClassName) jurisdiction.RetentionClass {
	retention := pack.Retention()
	if retention == nil {
		return jurisdiction.RetentionClass{}
	}
	for _, class := range retention.Classes() {
		if class.Name == name {
			return class
		}
	}
	return jurisdiction.RetentionClass{}
}

// CarryOnChangeTx records the floor a jurisdiction change is about to weaken,
// in the transaction that changes it. A change that keeps or raises the floor
// writes nothing. The stored carry keeps the strictest class ever in force,
// and its Before moves to now, so mail captured until now stays bound.
//
// Every change asks `retention_policy:update`, not only one that writes a
// carry: a jurisdiction change decides what may be destroyed either way.
func CarryOnChangeTx(ctx context.Context, tx pgx.Tx, store *settings.Store, from, to jurisdiction.Code, now time.Time) error {
	if err := auth.Require(ctx, retentionPolicyObject, principal.ActionUpdate); err != nil {
		return err
	}
	oldFloor, err := floorFor(from, now)
	if err != nil {
		return err
	}
	newFloor, err := floorFor(to, now)
	if err != nil {
		return err
	}
	if !newFloor.ProtectedSince(now).After(oldFloor.ProtectedSince(now)) {
		return nil
	}
	if err := settings.LockForWrite(ctx, tx, RetentionFloorCarry.Key()); err != nil {
		return err
	}
	stored, err := settings.ApplyTx(ctx, tx, RetentionFloorCarry)
	if err != nil {
		return err
	}
	keep := oldFloor
	if prior := stored.carried(); prior != nil && prior.Class.ProtectedSince(now).Before(keep.ProtectedSince(now)) {
		keep = prior.Class
	}
	raw, err := json.Marshal(carryOf(CarriedFloor{Class: keep, Before: now}))
	if err != nil {
		return fmt.Errorf("privacy: encoding the carried retention floor: %w", err)
	}
	return store.SetRawTx(ctx, tx, RetentionFloorCarry.Key(), raw)
}

// sqlArgs renders the policy as the two JSON arguments floorWindowEnd reads:
// the current floor, and the carried floor or NULL. Built by hand because each
// field is an ISO 8601 period, a bool or an RFC 3339 instant. So the encoding
// has no failure to report.
func (p FloorPolicy) sqlArgs() (current string, carried *string) {
	current = fmt.Sprintf(`{"keep":%q,"year_end":%t}`, p.Current.Keep.String(), p.Current.Anchor == jurisdiction.AnchorCalendarYearEnd)
	if p.Carried != nil {
		c := fmt.Sprintf(`{"keep":%q,"year_end":%t,"before":%q}`, p.Carried.Class.Keep.String(),
			p.Carried.Class.Anchor == jurisdiction.AnchorCalendarYearEnd, p.Carried.Before.UTC().Format(time.RFC3339Nano))
		carried = &c
	}
	return current, carried
}

// ShieldArgs are the two values StatutoryFloorShield's placeholders take.
func (p FloorPolicy) ShieldArgs() (current string, carried *string) { return p.sqlArgs() }

// archiveFloor is the policy an archive passes: archiving retains, so no
// floor applies to it.
var archiveFloor = FloorPolicy{}
