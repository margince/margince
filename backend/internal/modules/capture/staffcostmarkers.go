// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A document that names staff, each with a role and an amount.
//
// That is what a salary list and a cost-plus calculation look like. So does an
// invoice that bills each engineer by name at a monthly rate: it tells its
// reader what each of them costs. A model reads such an invoice as ordinary
// trade, because it is an invoice to a customer. This rule needs no model, so
// the verdict holds the thread whatever the model answers.
//
// It wants three such lines, or one that bills a named engineer with a rate
// and a total. A rate card that names roles and nobody does not match. A
// signature with a phone number has no rate and total.

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// minStaffCostLines is how many named staff with an amount make a list.
const minStaffCostLines = 3

// moneyPattern is a figure written with cents, as a rate or a total is.
var moneyPattern = regexp.MustCompile(`\b\d{1,3}(?:[.,]\d{3})*[.,]\d{2}\b`)

// amountPattern is a money-like figure: three or more digits, or grouped
// thousands, optionally with cents.
var amountPattern = regexp.MustCompile(`\b\d{1,3}(?:[.,]\d{3})+(?:[.,]\d{1,2})?\b|\b\d{3,}(?:[.,]\d{1,2})?\b`)

// roleWords are the job titles and stacks a staffing line names.
var roleWords = wordSet(`senior junior principal intern trainee developer developers engineer
	engineers frontend backend fullstack php java python qc qa pm designer tester consultant architect
	devops analyst manager scrum entwickler berater werkstudent`)

// notNameWords are capitalised words of a staffing line that are not a name.
var notNameWords = wordSet(`development devlopment services service standard project management product
	support sap erp additional income benefit monthly team building allowance payment pricing fixed model
	cost plus total balance due amount rate qty invoice level stack calculation sheet engineering mobile
	software web data quality assurance hours days month year lead staff`)

// ListsStaffCosts says whether text names three staff with an amount each, or
// bills one named engineer with a rate and a total.
//
// The text is cut into records at each line that holds an amount. A PDF table
// wraps a long name onto the next line before its figures.
func ListsStaffCosts(text string) bool {
	found := 0
	var record strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		record.WriteString(line)
		record.WriteByte(' ')
		if !amountPattern.MatchString(line) {
			continue
		}
		if namesStaffInARole(record.String()) {
			found++
			if found == minStaffCostLines || len(moneyPattern.FindAllString(record.String(), -1)) >= 2 {
				return true
			}
		}
		record.Reset()
	}
	return false
}

// namesStaffInARole says whether a record has a role word and two neighbouring
// capitalised words that are neither roles nor service words.
func namesStaffInARole(record string) bool {
	hasRole, run, named := false, 0, false
	for _, word := range strings.FieldsFunc(record, func(r rune) bool {
		return !unicode.IsLetter(r) && r != '+'
	}) {
		lower := strings.ToLower(strings.TrimRight(word, "+"))
		if roleWords[lower] {
			hasRole = true
		}
		if roleWords[lower] || notNameWords[lower] || !isCapitalised(word) {
			run = 0
			continue
		}
		run++
		named = named || run >= 2
	}
	return hasRole && named
}

// isCapitalised is a word that starts upper case and is not all upper case.
// So an acronym such as PHP is not read as a name.
func isCapitalised(word string) bool {
	runes := []rune(word)
	return len(runes) >= 2 && unicode.IsUpper(runes[0]) && slices.ContainsFunc(runes[1:], unicode.IsLower)
}

func wordSet(words string) map[string]bool {
	out := map[string]bool{}
	for w := range strings.FieldsSeq(words) {
		out[w] = true
	}
	return out
}
