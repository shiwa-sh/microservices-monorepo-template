// Command lint-pii asserts that a column holding personal data carries its
// `pii:<class>` comment in the migration set (ADR-0301, ADR-0300).
//
// The tag lives in the DDL so it travels with the schema and is readable from
// pg_description, which is what lets erasure, export, and redaction enumerate their
// targets by query rather than by memory. A tag nobody wrote is a row those
// workflows silently miss.
//
// # What a linter can and cannot decide
//
// It cannot know what is personal data. What it CAN do is refuse to let a column
// whose name is a well-known carrier of personal data pass without a DECISION —
// either a `pii:<class>` tag or an explicit `pii:none` saying it was considered and
// is not. Silence is the only outcome ruled out, because silence is
// indistinguishable from nobody having looked.
//
// The consequence is deliberate: this catches the obvious cases and gives the
// non-obvious ones no cover at all. A column named `notes` holding whatever a user
// typed is personal data, and no linter will say so. The data-class registry
// ADR-0301 names is the checklist for that.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Column-name fragments that carry personal data often enough that an untagged one
// is a review finding rather than a guess.
var piiIndicators = []string{
	"email", "phone", "mobile", "address", "postcode", "zip",
	"first_name", "last_name", "full_name", "given_name", "family_name", "surname",
	"user_id", "subject_id", "identity_id", "customer_id", "person",
	"dob", "birth", "national_id", "passport", "ssn", "tax_id",
	"ip_address", "user_agent", "device_id", "latitude", "longitude",
}

// Classes a tag may name. `none` is the explicit "considered, and it is not
// personal data" answer, which is what keeps the gate from being satisfied by
// mislabelling something.
var validClasses = []string{
	"none",
	"contact",          // email, phone, postal address
	"name",             // a person's name
	"identifier",       // a pseudonymous subject or account identifier
	"government_id",    // national identifier, passport, tax number
	"financial",        // payment instrument details
	"location",         // coordinates, precise geolocation
	"device",           // IP address, user agent, device identifier
	"special_category", // GDPR Art. 9 — health, biometrics, and the rest
	"free_text",        // a field a user types into, which may hold anything
}

var (
	createTableRe = regexp.MustCompile(
		`(?is)create\s+table\s+(?:if\s+not\s+exists\s+)?([a-z0-9_."]+)\s*\((.*?)\n\s*\)\s*;`,
	)
	commentRe = regexp.MustCompile(`(?i)comment\s+on\s+column\s+([a-z0-9_."]+)\.([a-z0-9_"]+)\s+is\s+'pii:([a-z_]+)'`)
	columnRe  = regexp.MustCompile(`(?i)^\s*([a-z0-9_"]+)\s+[a-z]`)
)

// A finding is one column that needs a decision, or one tag that is malformed.
type finding struct {
	file    string
	message string
}

func main() {
	migrations, err := filepath.Glob(filepath.Join("services", "*", "migrations", "*.sql"))
	if err != nil {
		failf("glob migrations: %v", err)
	}
	if len(migrations) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "✓ no migrations yet")
		return
	}
	sort.Strings(migrations)

	columns, tagged, findings := scan(migrations)

	for _, key := range sortedKeys(columns) {
		column := key[strings.LastIndex(key, ".")+1:]
		if !looksPersonal(column) {
			continue
		}
		_, isTagged := tagged[key]
		if isTagged {
			continue
		}
		const form = "%s holds personal data by its name and carries no pii: tag — add one, or pii:none if it does not"
		findings = append(findings, finding{columns[key], fmt.Sprintf(form, key)})
	}

	if len(findings) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, "✗ PII column tagging (ADR-0301):")
		for _, f := range findings {
			_, _ = fmt.Fprintf(os.Stderr, "  %s: %s\n", f.file, f.message)
		}
		os.Exit(1)
	}
	const summary = "✓ %d columns across %d migrations; every personal-data column is tagged\n"
	_, _ = fmt.Fprintf(os.Stdout, summary, len(columns), len(migrations))
}

// scan reads every migration once, collecting declared columns and pii tags.
//
// Tags are collected across the whole service, because the migration that creates a
// column and the one that later tags it need not be the same file — a dbmate
// migration is immutable once applied, so a retrospective tag is a NEW migration.
func scan(migrations []string) (map[string]string, map[string]string, []finding) {
	columns := map[string]string{} // "service:table.column" -> file that declares it
	tagged := map[string]string{}  // "service:table.column" -> class
	var findings []finding

	for _, path := range migrations {
		service := filepath.Base(filepath.Dir(filepath.Dir(path)))
		data, err := os.ReadFile(path)
		if err != nil {
			failf("read %s: %v", path, err)
		}
		sql := string(data)

		for _, m := range createTableRe.FindAllStringSubmatch(sql, -1) {
			table := unquote(m[1])
			for line := range strings.SplitSeq(m[2], "\n") {
				name := columnName(line)
				if name == "" {
					continue
				}
				columns[service+":"+table+"."+name] = path
			}
		}

		for _, m := range commentRe.FindAllStringSubmatch(sql, -1) {
			key := service + ":" + unquote(m[1]) + "." + unquote(m[2])
			class := m[3]
			if !slices.Contains(validClasses, class) {
				const form = "%s is tagged pii:%s, which is not a class — one of: %s"
				msg := fmt.Sprintf(form, key, class, strings.Join(validClasses, ", "))
				findings = append(findings, finding{path, msg})
				continue
			}
			tagged[key] = class
		}
	}
	return columns, tagged, findings
}

func looksPersonal(column string) bool {
	lower := strings.ToLower(column)
	for _, indicator := range piiIndicators {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

// columnName reads a column definition line, skipping table constraints.
func columnName(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "--") {
		return ""
	}
	lower := strings.ToLower(trimmed)
	for _, keyword := range []string{"primary key", "foreign key", "unique", "check", "constraint", "exclude"} {
		if strings.HasPrefix(lower, keyword) {
			return ""
		}
	}
	m := columnRe.FindStringSubmatch(trimmed)
	if m == nil {
		return ""
	}
	return unquote(m[1])
}

func unquote(s string) string {
	s = strings.ReplaceAll(s, `"`, "")
	// A schema-qualified name is stored under its table name alone: the migrations
	// declare one schema, and pg_description is queried by table.
	i := strings.LastIndex(s, ".")
	if i >= 0 {
		s = s[i+1:]
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func failf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
