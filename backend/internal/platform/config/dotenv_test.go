// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"reflect"
	"testing"
)

func TestDotenvReadsTheFormsOperatorsWrite(t *testing.T) {
	got, err := parseDotenv(".env", []byte(`# comment
X_A=plain
export X_B="quoted # not a comment"
X_C='single $NOT_EXPANDED'
X_D=trailing # comment

X_E=
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"X_A": "plain", "X_B": "quoted # not a comment",
		"X_C": "single $NOT_EXPANDED", "X_D": "trailing", "X_E": "",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDotenvNamesTheLineOfAMistake(t *testing.T) {
	_, err := parseDotenv("/run/x.env", []byte("X_A=1\nnot a pair\n"))
	if err == nil || err.Error() != "/run/x.env:2: want NAME=value" {
		t.Fatalf("got %v", err)
	}
	_, err = parseDotenv("x.env", []byte(`X_A="open`))
	if err == nil {
		t.Fatal("an unclosed quote must fail")
	}
}
