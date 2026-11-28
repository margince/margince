// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package licensecheck

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// verdict is one check's whole answer, so two checks compare as one value.
type verdict struct {
	result Result
	err    string
}

func checkVerdict(module *validator, authority, token string) verdict {
	result, err := module.check(context.Background(), authority, product, generation, token)
	return verdict{result: result, err: fmt.Sprint(err)}
}

// A check on the shared module answers as one on a module compiled for it alone.
// That holds however many checks ran before it or run beside it, so no instance
// leaves memory, output or an exit behind for the next.
func TestEveryCheckOnTheSharedModuleAnswersAsAFreshlyCompiledOne(t *testing.T) {
	t.Parallel()
	tokens := []string{
		"not-a-license",
		"a.b.c",
		"eyJhbGciOiJFZERTQSIsImtpZCI6IngifQ.eyJpc3MiOiJtYXJnaW5jZS1saWNlbnNlLWF1dGhvcml0eSJ9." + strings.Repeat("A", 86),
		craftedToken(t, `{"iss":"margince-license-authority","exp":9999999999,"iat":1,"pgs":{"margince":{"generation":0,"seats":5}}}`),
	}
	authorities := issuers(runtimeenv.Development)
	fresh := compiled(t, bundledModule)
	want := map[[2]string]verdict{}
	for _, authority := range authorities {
		for _, token := range tokens {
			want[[2]string{authority, token}] = checkVerdict(fresh, authority, token)
		}
	}

	shared := bundledValidator(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 2 {
				for key, expected := range want {
					if got := checkVerdict(shared, key[0], key[1]); !reflect.DeepEqual(got, expected) {
						t.Errorf("issuer %q, token %.20q: the shared module answered %+v, a fresh one %+v", key[0], key[1], got, expected)
					}
				}
			}
		})
	}
	wg.Wait()
}

// A stopped check closes its own instance and nothing more, so the check after
// it still gets the module's verdict.
func TestAStoppedCheckLeavesTheSharedModuleServingTheNext(t *testing.T) {
	t.Parallel()
	shared := bundledValidator(t)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, stop := range []struct {
		ctx  context.Context
		want error
	}{{expired(t), context.DeadlineExceeded}, {cancelled, context.Canceled}} {
		_, err := shared.check(stop.ctx, issuer, product, generation, "not-a-license")
		if !errors.Is(err, stop.want) || errors.Is(err, ErrVerdict) {
			t.Errorf("a stopped check = %v, want %v and no verdict", err, stop.want)
		}
		if _, err := shared.check(t.Context(), issuer, product, generation, "not-a-license"); !errors.Is(err, ErrVerdict) {
			t.Errorf("the check after a stopped one = %v, want the module's verdict", err)
		}
	}
}
