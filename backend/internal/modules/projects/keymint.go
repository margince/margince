// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Where a project's key comes from.
//
// A human writes the key in a subject line to file mail under a project, as
// in `[ERP-27] status` (capture/projectkeytoken.go). A key spelled as a
// colleague's name or an everyday word would file unrelated mail.
//
// So every key, chosen on create or minted from the name, passes one refusal
// (keychoice.go). After create the key never changes, so sent mail keeps filing.

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// keyStemMaxLen bounds the letters taken from the name. The whole key must fit
// project_key_shape (letter-led, 2 to 24 characters), and the suffix below adds
// a hyphen plus digits, so the stem stops well short of the ceiling.
const keyStemMaxLen = 8

// projectKeyField is the body field a caller may send on create and never on
// update.
const projectKeyField = "key"

// keyMintAttempts bounds the retry loop. Each attempt asks for the next free
// number for one stem, so an attempt is only spent when ANOTHER transaction
// committed that number in between — a race that needs two creates of the same
// stem in the same instant. The bound exists so a bug cannot spin forever; it
// is set well above the number of simultaneous creates of one stem any human
// workflow produces, because exhausting it means a 500 for a caller who did
// nothing wrong.
const keyMintAttempts = 12

// keySuffixCeiling is the largest number this generator will mint, and the
// largest it will read back out of an existing key.
//
// It is here because the suffix is read with a cast to a Postgres int: a key
// the OLD contract allowed a caller to choose ("AB-9999999999") would overflow
// that cast and fail every create of a project whose name gives the stem AB.
// Numbers above the ceiling are ignored rather than read, so such a key blocks
// its own number and nothing else.
const keySuffixCeiling = 100000

// keyInsertLockTimeout bounds how long ONE insert attempt waits on the unique
// index before giving the number up.
//
// Without it a create can wait indefinitely: another transaction that inserted
// the same candidate and has not committed holds the index entry, and the
// waiter holds its pool connection for as long as that lasts. Enough of those
// and the pool is gone — for a key nobody chose. With the bound, the wait
// becomes a lost race, which the loop already knows how to answer: take the
// next number.
//
// A compile-time literal because a GUC value that could come from a request is
// a GUC value a request can set to "0".
const keyInsertLockTimeout = "750ms"

// keyFallbackStem is what a name with no usable letters becomes — a name in a
// script this transliteration does not cover, or one made entirely of
// punctuation. "PRJ" keeps such a project addressable in a subject line rather
// than leaving it with no handle at all.
const keyFallbackStem = "PRJ"

// keyStem reduces a project name to the letters its key is built from: the
// initials of the first words when the name has several, otherwise the opening
// letters of the single word. "ERP rollout Acme" gives ERA, "Datenmigration"
// gives DATENMIG.
//
// Only ASCII letters and digits survive, because the key travels through mail
// subjects, and a matcher whose characters depend on the reader's encoding is a
// matcher that silently stops matching.
func keyStem(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var stem strings.Builder
	for _, word := range words {
		for _, r := range word {
			if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r)) {
				continue
			}
			stem.WriteRune(unicode.ToUpper(r))
			break
		}
	}
	// A single word gives one initial, which is too thin to recognise in a
	// subject line, so it contributes its opening letters instead.
	if len(words) == 1 {
		stem.Reset()
		for _, r := range words[0] {
			if stem.Len() == keyStemMaxLen {
				break
			}
			if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r)) {
				continue
			}
			stem.WriteRune(unicode.ToUpper(r))
		}
	}
	// The shape demands a leading LETTER, and a name can open with a digit
	// ("7 Eleven Rollout", "2026 migration"). Dropping the leading digits keeps
	// the letters that make the key recognisable rather than throwing the whole
	// stem away for its first character.
	out := strings.TrimLeftFunc(stem.String(), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	if len(out) > keyStemMaxLen {
		out = out[:keyStemMaxLen]
	}
	// Under two characters there is nothing left to recognise, so the fallback
	// carries it instead.
	if len(out) < 2 {
		return keyFallbackStem
	}
	return out
}

// mintProjectKey answers the key a new project gets when its creator chose
// none. It is the bare stem when no live project uses those letters and the
// refusal admits the stem. Otherwise it is the stem plus the lowest free number.
//
// The numbers are read from the live keys, not from a counter, because
// archiving frees a key (uq_project_key is partial on archived_at).
// The read is unscoped, as the uniqueness index is; only the free key leaves.
func mintProjectKey(ctx context.Context, tx pgx.Tx, name string, taken map[string]bool) (string, error) {
	stem := keyStem(name)
	inUse, bareLive, err := stemKeysInUse(ctx, tx, stem)
	if err != nil {
		return "", err
	}
	bareHeld := bareLive || taken[stem]
	if !bareHeld && len(inUse) == 0 {
		refusal, err := keyWordRefusal(ctx, tx, stem)
		if err != nil {
			return "", err
		}
		if refusal == nil {
			return stem, nil
		}
	}
	// Next to a live bare `ERA` the numbers start at 2, so the two read as
	// first and second.
	first := 1
	if bareHeld {
		first = 2
	}
	// The lowest free number, so a number an archived project gave back is
	// reused. `taken` holds keys this transaction minted but has not inserted.
	for next := first; next <= keySuffixCeiling; next++ {
		candidate := fmt.Sprintf("%s-%d", stem, next)
		if !inUse[next] && !taken[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("every key number up to %d is in use for the stem %q", keySuffixCeiling, stem)
}

// stemKeysInUse reads which numbers live projects hold on one stem, and
// whether one holds the bare stem itself.
func stemKeysInUse(ctx context.Context, tx pgx.Tx, stem string) (map[int]bool, bool, error) {
	// Case-insensitive, because uq_project_key indexes lower(key).
	// The suffix is capped at six digits so the int cast cannot overflow.
	rows, err := tx.Query(ctx,
		`SELECT substring(lower(key) from '^'||lower($1)||'-([0-9]{1,6})$')::int
		   FROM project
		  WHERE lower(key) ~ ('^'||lower($1)||'(-[0-9]{1,6})?$') AND archived_at IS NULL`,
		stem)
	if err != nil {
		return nil, false, fmt.Errorf("read the keys in use for stem %q: %w", stem, err)
	}
	defer rows.Close()
	inUse := map[int]bool{}
	bare := false
	for rows.Next() {
		var n *int
		if err := rows.Scan(&n); err != nil {
			return nil, false, fmt.Errorf("read one key for stem %q: %w", stem, err)
		}
		if n == nil {
			bare = true
			continue
		}
		inUse[*n] = true
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read the keys in use for stem %q: %w", stem, err)
	}
	return inUse, bare, nil
}

// insertProjectRow writes the project with the key its creator chose, or with
// a minted one.
//
// A minted key that another transaction takes first is retried with the next
// key, because the caller never chose it. A chosen key that loses the same race
// is refused as taken. Every other refusal is returned as it stands.
func insertProjectRow(
	ctx context.Context, tx pgx.Tx, id ids.ProjectID,
	in CreateProjectInput, by string, active []fieldcatalog.Column,
) (string, error) {
	// Checked once, so a refused custom field never opens a savepoint.
	if err := storekit.CheckCustomValues(active, in.CustomFields); err != nil {
		return "", err
	}
	if in.Key != nil {
		return insertChosenKey(ctx, tx, id, *in.Key, in, by, active)
	}
	minted := map[string]bool{}
	var lastErr error
	for range keyMintAttempts {
		key, err := mintProjectKey(ctx, tx, in.Name, minted)
		if err != nil {
			return "", err
		}
		minted[key] = true
		inserted, err := insertOnce(ctx, tx, id, key, in, by, active)
		if inserted {
			return key, nil
		}
		if err == nil {
			return "", fmt.Errorf("insert project %q: the write neither succeeded nor refused", in.Name)
		}
		if !keyRaceLost(err) {
			return "", insertRefusal(err, in)
		}
		lastErr = err
	}
	return "", fmt.Errorf("mint a free key for %q after %d attempts: %w", in.Name, keyMintAttempts, lastErr)
}

// insertChosenKey writes the project under the key admitProjectCreation already
// held to the refusal, answering a lost race on it as the key being taken.
func insertChosenKey(
	ctx context.Context, tx pgx.Tx, id ids.ProjectID, key string,
	in CreateProjectInput, by string, active []fieldcatalog.Column,
) (string, error) {
	inserted, err := insertOnce(ctx, tx, id, key, in, by, active)
	switch {
	case inserted:
		return key, nil
	case err == nil:
		return "", fmt.Errorf("insert project %q: the write neither succeeded nor refused", in.Name)
	case keyRaceLost(err):
		return "", keyTaken(key)
	default:
		return "", insertRefusal(err, in)
	}
}

// insertOnce runs one INSERT attempt inside a SAVEPOINT, because a failed
// statement aborts the transaction it ran in and the next attempt needs a live
// one — the same shape and the same reason as activities' message-identity
// re-key and capture's guarded counterparty decision.
//
// It reports whether the row landed, separately from the error, so a caller
// cannot read "no error" as "written" when the savepoint itself failed to
// commit.
func insertOnce(
	ctx context.Context, tx pgx.Tx, id ids.ProjectID, key string,
	in CreateProjectInput, by string, active []fieldcatalog.Column,
) (bool, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("open the project-insert savepoint: %w", err)
	}
	// Set INSIDE the savepoint, so a rollback takes the setting with it and the
	// writes that follow this one keep whatever bound their caller chose. The
	// audit and outbox rows want to WAIT for an unrelated lock rather than
	// abandon a create over one.
	if _, err := sp.Exec(ctx, `SELECT set_config('lock_timeout', $1, true)`, keyInsertLockTimeout); err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return false, fmt.Errorf("roll the project-insert savepoint back after %v: %w", err, rbErr)
		}
		return false, fmt.Errorf("bound the project-insert lock wait: %w", err)
	}
	provenanceCols, provenanceHolders, base := storekit.ProvenanceInsertFragments(in.SourceID, in.Author, []any{
		id, in.Name, key, in.CompanyID, in.OwnerID,
		in.Description, in.StartedAt, in.TargetEndDate, in.Source, by,
		in.SourceSystem, in.StartPhase(),
	})
	cfCols, cfHolders, args, cfErr := storekit.InsertFragments(active, in.CustomFields, base)
	if cfErr != nil {
		return false, cfErr
	}
	insertErr := storekit.ExecSourceInsert(ctx, sp, projectObject,
		`INSERT INTO project (id, name, key, company_id, owner_id,
		                      description, started_at, target_end_date, source, captured_by, source_system, phase`+provenanceCols+cfCols+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12`+provenanceHolders+cfHolders+`)`,
		args)
	if insertErr != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return false, fmt.Errorf("roll the project-insert savepoint back after %v: %w", insertErr, rbErr)
		}
		return false, insertErr
	}
	if err := sp.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit the project insert: %w", err)
	}
	return true, nil
}

// insertRefusal maps an insert error that is NOT a key race onto the typed
// refusal the caller gets, so every create path answers one way.
func insertRefusal(err error, in CreateProjectInput) error {
	// Covers the owner FK; the company target was pre-checked.
	if storekit.IsForeignKeyViolation(err) {
		return apperrors.ErrNotFound
	}
	if constraint, ok := storekit.CheckViolation(err); ok {
		if refusal := projectCheckError(constraint, submittedDateField(in.StartedAt, in.TargetEndDate, nil)); refusal != nil {
			return refusal
		}
	}
	return fmt.Errorf("insert project: %w", err)
}
