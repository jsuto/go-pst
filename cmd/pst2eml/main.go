// go-pst is a library for reading Personal Storage Table (.pst) files (written in Go/Golang).
//
// Copyright 2026 Janos SUTO
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// pst2eml converts the messages of a PST file to EML files, one directory per folder, and prints a report per
// folder: items read, converted and skipped (with the reason).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pst "github.com/jsuto/go-pst/pkg"
	"github.com/jsuto/go-pst/pkg/eml"
)

// folderReport counts the items of a folder.
type folderReport struct {
	path               string
	inDeletedItems     bool
	read               int
	converted          int
	skipped            map[string]int
	errors             int
	skippedAttachments int
}

func main() {
	outputDirectory := flag.String("o", "", "output directory (required)")
	codePage := flag.Int("codepage", pst.DefaultCodePage, "code page of 8-bit strings without PR_MESSAGE_CODEPAGE (ANSI files)")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s -o <directory> [options] <file.pst>\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}

	flag.Parse()

	if *outputDirectory == "" || flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(flag.Arg(0), *outputDirectory, *codePage); err != nil {
		fmt.Fprintf(os.Stderr, "pst2eml: %v\n", err)
		os.Exit(1)
	}
}

func run(pstPath string, outputDirectory string, codePage int) error {
	reader, err := os.Open(pstPath)

	if err != nil {
		return err
	}

	defer reader.Close()

	pstFile, err := pst.New(reader)

	if err != nil {
		return fmt.Errorf("failed to open %s: %w", pstPath, err)
	}

	defer pstFile.Cleanup()

	pstFile.DefaultCodePage = codePage

	reports := map[string]*folderReport{}
	var folderOrder []string
	var buffer bytes.Buffer

	walkErr := eml.Walk(pstFile, func(item eml.Item) error {
		folderPath := strings.Join(item.FolderPath, "/")
		report, ok := reports[folderPath]

		if !ok {
			report = &folderReport{path: folderPath, inDeletedItems: item.InDeletedItems, skipped: map[string]int{}}
			reports[folderPath] = report
			folderOrder = append(folderOrder, folderPath)
		}

		if item.Err != nil {
			report.errors++
			fmt.Fprintf(os.Stderr, "%s: message %d: %v\n", displayPath(folderPath), item.MessageIdentifier, item.Err)

			return nil
		}

		report.read++
		buffer.Reset()

		result, err := eml.Convert(&buffer, item.Message)

		var skipError *eml.SkipError

		switch {
		case errors.As(err, &skipError):
			report.skipped[skipError.Kind.String()]++

			return nil
		case err != nil:
			report.errors++
			fmt.Fprintf(os.Stderr, "%s: message %d: %v\n", displayPath(folderPath), item.MessageIdentifier, err)

			return nil
		}

		for _, skippedAttachment := range result.SkippedAttachments {
			report.skippedAttachments++
			fmt.Fprintf(os.Stderr, "%s: message %d: %s\n", displayPath(folderPath), item.MessageIdentifier, skippedAttachment)
		}

		directory := filepath.Join(append([]string{outputDirectory}, sanitizePath(item.FolderPath)...)...)

		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}

		report.converted++

		return os.WriteFile(filepath.Join(directory, fmt.Sprintf("%d.eml", report.converted)), buffer.Bytes(), 0o644)
	})

	printReport(reports, folderOrder)

	return walkErr
}

// sanitizePath makes folder names usable as directory names.
func sanitizePath(folderPath []string) []string {
	sanitized := make([]string, len(folderPath))

	for i, name := range folderPath {
		name = strings.Map(func(character rune) rune {
			if character < ' ' || strings.ContainsRune(`/\:*?"<>|`, character) {
				return '_'
			}

			return character
		}, strings.TrimSpace(name))

		if name == "" || name == "." || name == ".." {
			name = "_"
		}

		sanitized[i] = name
	}

	return sanitized
}

func displayPath(folderPath string) string {
	if folderPath == "" {
		return "(top)"
	}

	return folderPath
}

func printReport(reports map[string]*folderReport, folderOrder []string) {
	var total folderReport

	total.skipped = map[string]int{}

	for _, folderPath := range folderOrder {
		report := reports[folderPath]

		fmt.Printf("%s%s: %s\n", displayPath(folderPath), deletedItemsMark(report.inDeletedItems), formatCounts(report))

		total.read += report.read
		total.converted += report.converted
		total.errors += report.errors
		total.skippedAttachments += report.skippedAttachments

		for reason, count := range report.skipped {
			total.skipped[reason] += count
		}
	}

	fmt.Printf("total: %s\n", formatCounts(&total))
}

func deletedItemsMark(inDeletedItems bool) string {
	if inDeletedItems {
		return " [deleted items]"
	}

	return ""
}

func formatCounts(report *folderReport) string {
	counts := fmt.Sprintf("read %d, converted %d", report.read, report.converted)

	var reasons []string

	for reason := range report.skipped {
		reasons = append(reasons, reason)
	}

	sort.Strings(reasons)

	for _, reason := range reasons {
		counts += fmt.Sprintf(", skipped %d (%s)", report.skipped[reason], reason)
	}

	if report.errors > 0 {
		counts += fmt.Sprintf(", errors %d", report.errors)
	}

	if report.skippedAttachments > 0 {
		counts += fmt.Sprintf(", attachments skipped %d", report.skippedAttachments)
	}

	return counts
}
