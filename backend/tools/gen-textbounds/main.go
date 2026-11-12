// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

func main() {
	output := flag.String("output", "", "write a new migration pair at this stem")
	flag.Parse()
	if err := generate(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(output string) error {
	if output != "" {
		catalog, err := os.ReadFile("migrations/testdata/head_catalog.txt")
		if err != nil {
			return err
		}
		up, down, err := textbounds.SQL(bytes.NewReader(catalog))
		if err != nil {
			return err
		}
		if err := writeNew(output+".up.sql", up); err != nil {
			return err
		}
		if err := writeNew(output+".down.sql", down); err != nil {
			return err
		}
	}
	return os.WriteFile("../frontend/src/lib/textlimits_gen.ts", []byte(textbounds.Frontend()), 0644)
}

func writeNew(path, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	return errors.Join(writeErr, file.Close())
}
