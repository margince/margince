// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Where a project's key comes from.
//
// The key is what a human writes in a subject line to file a mail under a
// project — `[ERP-27] status`, matched by capture/projectkeytoken.go. That makes
// it a MATCHER, and a matcher a user types is a matcher a user gets wrong: a
// project keyed after its owner ("Lars") reads every bracketed mention of that
// word as an attribution, onto records that are then stamped for six years of
// retention. Nobody sets out to build that; they name a project after
// themselves and the consequence arrives later, invisibly.
//
// So the server mints it. The name gives the letters, a counter gives the
// number, and the caller never chooses either: the field is absent from the
// create and update bodies, which is what makes "read-only" a property of the
// contract rather than a promise the UI keeps.

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

// projectKeyField is the body field a caller may no longer send, named once so
// the refusal and the check that raises it cannot drift apart.
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

// mintProjectKey answers the key a new project gets: its stem plus the lowest
// number that stem is not already using.
//
// The number is read from the keys that EXIST rather than from a counter,
// because a counter is a second piece of state that can disagree with the
// column it numbers — an archived project still holds its key (uq_project_key
// is partial on archived_at, so archiving frees it), and a restore would then
// collide with whatever took the number meanwhile. Reading max+1 from the live
// rows cannot disagree with itself.
//
// The read is unscoped on purpose: the uniqueness index it feeds is unscoped,
// so a key hidden from this caller still blocks the number. Nothing about the
// hidden project leaves this function — only the next free integer does.
func mintProjectKey(ctx context.Context, tx pgx.Tx, name string, taken map[string]bool) (string, error) {
	stem := keyStem(name)
	// Case-INSENSITIVE, because uq_project_key indexes lower(key): a live key
	// spelled "ab-3" blocks "AB-3", so a case-sensitive read would hand back a
	// number the index then refuses. Keys the old contract let callers choose
	// are exactly where that mismatch lives.
	//
	// The suffix is capped before the cast: a key with a number larger than a
	// Postgres int would otherwise abort this statement, and with it every
	// create of a project whose name gives this stem.
	rows, err := tx.Query(ctx,
		`SELECT substring(lower(key) from '^'||lower($1)||'-([0-9]{1,6})$')::int
		   FROM project
		  WHERE lower(key) ~ ('^'||lower($1)||'-[0-9]{1,6}$') AND archived_at IS NULL`,
		stem)
	if err != nil {
		return "", fmt.Errorf("read the numbers in use for key stem %q: %w", stem, err)
	}
	inUse := map[int]bool{}
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return "", fmt.Errorf("read one key number for stem %q: %w", stem, err)
		}
		inUse[n] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read the numbers in use for key stem %q: %w", stem, err)
	}
	// The LOWEST free number rather than max+1, so a number an archived project
	// gave back is reused instead of leaving a permanent hole. The keys this
	// same transaction already minted but has not inserted are in `taken`,
	// which the statement above cannot see.
	for next := 1; next <= keySuffixCeiling; next++ {
		candidate := fmt.Sprintf("%s-%d", stem, next)
		if !inUse[next] && !taken[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("every key number up to %d is in use for the stem %q", keySuffixCeiling, stem)
}

// insertProjectRow writes the project with a minted key, retrying when another
// transaction takes the same key between the mint and the insert.
//
// The retry is what makes a server-minted key honest: the caller did not choose
// it, so a collision is the server's own race to resolve, not a 409 to hand
// back. Only the unique-key violation retries — every other refusal is about
// what the caller sent and is returned as it stands.
//
// Each attempt runs in a savepoint, because a failed INSERT aborts the
// transaction it ran in and the next attempt needs a live one.
func insertProjectRow(
	ctx context.Context, tx pgx.Tx, id ids.ProjectID,
	in CreateProjectInput, by string, active []fieldcatalog.Column,
) (string, error) {
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
	cfCols, cfHolders, args := storekit.InsertFragments(active, in.CustomFields, []any{
		id, in.Name, key, in.OrganizationID, in.OwnerID,
		in.Description, in.StartedAt, in.TargetEndDate, in.Source, by,
	})
	_, insertErr := sp.Exec(ctx,
		`INSERT INTO project (id, name, key, organization_id, owner_id,
		                      description, started_at, target_end_date, source, captured_by`+cfCols+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10`+cfHolders+`)`,
		args...)
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
	// Covers the owner FK; the organization target was pre-checked.
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
