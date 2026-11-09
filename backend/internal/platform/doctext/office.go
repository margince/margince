// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package doctext

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"
)

// readWorkbook writes every sheet in workbook order, one line per row, with a
// cell's shared-string index resolved to its value. A row is kept whole
// because the sensitive fact is the pair: a name beside an amount.
func readWorkbook(ctx context.Context, parts *archiveParts, out *excerptWriter) error {
	shared, err := sharedStrings(ctx, parts)
	if err != nil {
		return err
	}
	sheets, err := workbookSheets(ctx, parts)
	if err != nil {
		return err
	}
	for _, sheet := range sheets {
		if err := readSheet(ctx, parts, sheet, shared, out); err != nil {
			return err
		}
	}
	return nil
}

// sharedStrings reads the workbook's string dictionary. A workbook with no
// dictionary keeps its strings inline, which is not an error.
func sharedStrings(ctx context.Context, parts *archiveParts) ([]string, error) {
	const name = "xl/sharedStrings.xml"
	if !parts.has(name) {
		return nil, nil
	}
	var (
		out     []string
		current strings.Builder
		inText  bool
	)
	err := walkPart(ctx, parts, name, func(tok xml.Token) error {
		switch t := tok.(type) {
		case xml.StartElement:
			inText = t.Name.Local == "t"
		case xml.EndElement:
			inText = false
			if t.Name.Local == "si" {
				if len(out) == maxSharedStrings {
					return fmt.Errorf("%w: the workbook holds more than %d strings", ErrUnreadable, maxSharedStrings)
				}
				out = append(out, current.String())
				current.Reset()
			}
		case xml.CharData:
			if inText {
				current.Write(t)
			}
		}
		return nil
	})
	return out, err
}

// sheetPart is one worksheet: the name a reader sees and the archive entry
// that holds its cells.
type sheetPart struct {
	name string
	part string
}

// workbookSheets lists the worksheets in the order the workbook declares them.
// Without followable relationships it falls back to the worksheet entries in
// name order, so no sheet is skipped for want of an index.
func workbookSheets(ctx context.Context, parts *archiveParts) ([]sheetPart, error) {
	targets, err := workbookTargets(ctx, parts)
	if err != nil {
		return nil, err
	}
	var sheets []sheetPart
	err = walkPart(ctx, parts, "xl/workbook.xml", func(tok xml.Token) error {
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "sheet" {
			return nil
		}
		var name, rel string
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "name":
				name = a.Value
			case "id":
				rel = a.Value
			}
		}
		if part, ok := targets[rel]; ok && parts.has(part) {
			sheets = append(sheets, sheetPart{name: name, part: part})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(sheets) > 0 {
		return sheets, nil
	}
	for name := range parts.files {
		if strings.HasPrefix(name, "xl/worksheets/") && strings.HasSuffix(name, ".xml") {
			sheets = append(sheets, sheetPart{name: path.Base(name), part: name})
		}
	}
	slices.SortFunc(sheets, func(a, b sheetPart) int { return strings.Compare(a.part, b.part) })
	return sheets, nil
}

// workbookTargets maps each relationship id to the archive entry it names.
func workbookTargets(ctx context.Context, parts *archiveParts) (map[string]string, error) {
	const name = "xl/_rels/workbook.xml.rels"
	targets := map[string]string{}
	if !parts.has(name) {
		return targets, nil
	}
	err := walkPart(ctx, parts, name, func(tok xml.Token) error {
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			return nil
		}
		var id, target string
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "Id":
				id = a.Value
			case "Target":
				target = a.Value
			}
		}
		// A target is relative to xl/ unless it is rooted at the archive.
		if rooted, ok := strings.CutPrefix(target, "/"); ok {
			targets[id] = path.Clean(rooted)
		} else {
			targets[id] = path.Clean("xl/" + target)
		}
		return nil
	})
	return targets, err
}

// readSheet writes one worksheet's non-empty rows, cells joined by " | ",
// under the sheet's name. An empty sheet writes nothing.
func readSheet(ctx context.Context, parts *archiveParts, sheet sheetPart, shared []string, out *excerptWriter) error {
	header := "Sheet: " + sheet.name + "\n"
	var (
		row       []string
		cell      strings.Builder
		cellType  string
		inValue   bool
		inlineTxt bool
	)
	return walkPart(ctx, parts, sheet.part, func(tok xml.Token) error {
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "c":
				cellType, inValue, inlineTxt = attr(t, "t"), false, false
				cell.Reset()
			case "v":
				inValue = true
			case "t":
				inlineTxt = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inValue = false
			case "t":
				inlineTxt = false
			case "c":
				if value := cellValue(cellType, cell.String(), shared); value != "" {
					row = append(row, value)
				}
			case "row":
				line := strings.Join(row, " | ")
				row = row[:0]
				if line == "" {
					return nil
				}
				line, header = header+line, ""
				return out.write(line + "\n")
			}
		case xml.CharData:
			if inValue || inlineTxt {
				cell.Write(t)
			}
		}
		return nil
	})
}

// cellValue resolves a shared-string cell to its string and keeps every other
// cell's value as written.
func cellValue(cellType, raw string, shared []string) string {
	raw = strings.TrimSpace(raw)
	if cellType != "s" {
		return raw
	}
	i, err := strconv.Atoi(raw)
	if err != nil || i < 0 || i >= len(shared) {
		return ""
	}
	return strings.TrimSpace(shared[i])
}

// readDocument writes a word-processing document's paragraphs, one per line.
func readDocument(ctx context.Context, parts *archiveParts, out *excerptWriter) error {
	inText := false
	return walkPart(ctx, parts, "word/document.xml", func(tok xml.Token) error {
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				return out.write("\t")
			case "br":
				return out.write("\n")
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				return out.write("\n")
			}
		case xml.CharData:
			if inText {
				return out.write(string(t))
			}
		}
		return nil
	})
}

// attr is the value of the attribute named local, or "".
func attr(start xml.StartElement, local string) string {
	for _, a := range start.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// walkPart streams one archive entry's XML tokens to visit, checking the
// caller's context as it goes. A visit error ends the walk and is returned
// as is, so errFull reaches the caller unchanged.
func walkPart(ctx context.Context, parts *archiveParts, name string, visit func(xml.Token) error) (err error) {
	rc, err := parts.open(name)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rc.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("%w: %w", ErrUnreadable, closeErr)
		}
	}()
	decoder := xml.NewDecoder(rc)
	for n := 0; ; n++ {
		if n%cancelEvery == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, errBudget) {
			return errBudget
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrUnreadable, err)
		}
		if err := visit(tok); err != nil {
			return err
		}
	}
}
