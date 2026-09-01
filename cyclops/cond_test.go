package cyclops

import "fmt"
import "maps"
import "os"
import "path/filepath"
import "slices"
import "strings"
import "testing"

// corpusSchema declares the fields and filters used by the test corpus in ramls/examples/condtest.
func corpusSchema() *CondSchema {
	return &CondSchema{
		Fields: map[string]FieldKind{
			"title":          FieldString,
			"author":         FieldString,
			"note":           FieldString,
			"availability":   FieldString,
			"location":       FieldString,
			"acquired":       FieldDate,
			"withdrawn_date": FieldDate,
			"holdings_count": FieldNumber,
			"decision":       FieldBoolean,
		},
		Filters: map[string]bool{
			"target":   true,
			"reviewed": true,
		},
	}
}

// Every file in ramls/examples/condtest/valid must render, and the RAML example with it.
func TestCondCorpusValid(t *testing.T) {
	files, err := filepath.Glob("../ramls/examples/condtest/valid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, "../ramls/examples/cond.json")
	if len(files) < 2 {
		t.Fatalf("found %d valid test cases; the corpus has gone missing", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			_, err := ParseCond(readCase(t, f), corpusSchema())
			if err != nil {
				t.Fatalf("wrongly rejected: %v", err)
			}
			// t.Logf("=> %s", cond)
		})
	}
}

// Every file in ramls/examples/condtest/invalid must be rejected, and rejected for the
// reason it was written to probe: a case that starts failing for some unrelated
// reason has quietly stopped testing anything. The expected text need only be a
// distinctive fragment of the error.
var invalidCases = map[string]string{
	"bad-field-name.json":            `field is not queryable: "a; drop table root"`,
	"ccms-operator-as-rel.json":      `unknown relation ">="`,
	"empty-field-name.json":          `term has no "field"`,
	"empty-junction.json":            `"and" clause has no subordinate clauses`,
	"extra-property.json":            `unknown field "cond"`,
	"filter-empty-name.json":         `filter reference has no "name"`,
	"filter-missing-name.json":       `filter reference has no "name"`,
	"in-given-empty-list.json":       `relation "in" needs a non-empty list of values`,
	"in-given-nested-list.json":      `value[0]: not a string, number or boolean`,
	"in-given-scalar.json":           `relation "in" needs a list of values`,
	"junction-missing-clauses.json":  `"and" clause has no subordinate clauses`,
	"missing-discriminator.json":     `clause has no "type"`,
	"missing-value.json":             `relation "eq" needs a value`,
	"nested-bad-clause.json":         `condition.clauses[0]: unknown relation "nope"`,
	"nested-injection-as-field.json": `field is not queryable: "1=1 or a"`,
	"not-an-object.json":             `not a condition clause`,
	"not-missing-clause.json":        `"not" clause has no subordinate clause`,
	"not-with-clauses.json":          `unknown field "clauses"`,
	"null-value.json":                `value is null (use the "isNull" relation)`,
	"scalar-rel-given-array.json":    `value: not a string, number or boolean`,
	"term-missing-field.json":        `term has no "field"`,
	"unknown-rel.json":               `unknown relation "ilike"`,
	"unknown-type.json":              `unknown clause type "xyzzy"`,
	"value-with-is-null.json":        `relation "isNull" takes no value`,
}

func TestCondCorpusInvalid(t *testing.T) {
	files, err := filepath.Glob("../ramls/examples/condtest/invalid/*.json")
	if err != nil {
		t.Fatal(err)
	}
	// The corpus and the expectations must name exactly the same cases, so that
	// neither a new fixture nor a deleted one can slip through unnoticed.
	found := make(map[string]bool, len(files))
	for _, f := range files {
		name := filepath.Base(f)
		found[name] = true
		if _, ok := invalidCases[name]; !ok {
			t.Errorf("%s: test case has no expected error in invalidCases", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(invalidCases)) {
		if !found[name] {
			t.Errorf("%s: expected error in invalidCases, but no such test case", name)
		}
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			want, ok := invalidCases[name]
			if !ok {
				t.Fatalf("no expected error recorded for this case")
			}
			cond, err := ParseCond(readCase(t, f), corpusSchema())
			if err == nil {
				t.Fatalf("wrongly accepted, rendering as: %s", cond)
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("rejected for the wrong reason:\n  got  %v\n  want it to contain %q", err, want)
			}
			// t.Logf("rejected: %v", err)
		})
	}
}

// A Junction is exported with exported fields, so a tree can be built in Go
// rather than decoded from JSON. Rendering must therefore validate the operator
// itself: it may not simply write whatever Op holds into the command. Only the
// operators in junctionOps are admissible, and what is written is the keyword
// the table maps to, never the caller's string.
func TestJunctionOperatorNotInjectable(t *testing.T) {
	build := func(op string) *Junction {
		return &Junction{
			Op: op,
			Clauses: []Clause{
				&FilterRef{Name: "target"},
				&FilterRef{Name: "reviewed"},
			},
		}
	}

	for op, want := range map[string]string{
		"and": "(filter(target) and filter(reviewed))",
		"or":  "(filter(target) or filter(reviewed))",
	} {
		got, err := RenderCond(build(op), corpusSchema())
		if err != nil {
			t.Errorf("Op=%q unexpectedly rejected: %v", op, err)
		} else if got != want {
			t.Errorf("Op=%q rendered as %q, want %q", op, got, want)
		}
	}

	// Each of these would break out of the operator position, or is simply not
	// an operator, and must be refused rather than written into the command.
	for _, op := range []string{
		"",
		"; drop set root; --",
		"and 1=1 and",
		"AND",
		"not",
		")",
	} {
		got, err := RenderCond(build(op), corpusSchema())
		if err == nil {
			t.Errorf("Op=%q should have been rejected, rendered as %q", op, got)
			continue
		}
		if want := fmt.Sprintf("unknown junction operator %q", op); !strings.Contains(err.Error(), want) {
			t.Errorf("Op=%q error = %q, want it to contain %q", op, err.Error(), want)
		}
		if got != "" {
			t.Errorf("Op=%q returned %q alongside its error; want no condition at all", op, got)
		}
	}
}

// The three limits on the size of a condition tree. None of these can be
// expressed as a corpus fixture without checking in a very large file, so they
// are exercised by building the JSON here.
func TestCondLimits(t *testing.T) {
	term := `{"type":"term","field":"title","rel":"eq","value":"x"}`

	nest := func(n int) string {
		doc := term
		for range n {
			doc = `{"type":"not","clause":` + doc + `}`
		}
		return doc
	}
	widen := func(n int) string {
		clauses := make([]string, n)
		for i := range clauses {
			clauses[i] = term
		}
		return `{"type":"and","clauses":[` + strings.Join(clauses, ",") + `]}`
	}
	list := func(n int) string {
		values := make([]string, n)
		for i := range values {
			values[i] = `"x"`
		}
		return `{"type":"term","field":"title","rel":"in","value":[` + strings.Join(values, ",") + `]}`
	}

	cases := []struct {
		name string
		doc  string
		want string // fragment of the expected error, or "" if it must be accepted
	}{
		{"depth within limit", nest(maxCondDepth - 1), ""},
		{"depth over limit", nest(maxCondDepth + 1), fmt.Sprintf("nested more than %d deep", maxCondDepth)},
		{"nodes within limit", widen(maxCondNodes - 1), ""},
		{"nodes over limit", widen(maxCondNodes + 1), fmt.Sprintf("more than %d clauses", maxCondNodes)},
		{"list within limit", list(maxCondListValues), ""},
		{"list over limit", list(maxCondListValues + 1), fmt.Sprintf(`relation "in" has more than %d values`, maxCondListValues)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseCond([]byte(c.doc), corpusSchema())
			switch {
			case c.want == "" && err != nil:
				t.Errorf("unexpectedly rejected: %v", err)
			case c.want != "" && err == nil:
				t.Errorf("unexpectedly accepted")
			case c.want != "" && !strings.Contains(err.Error(), c.want):
				t.Errorf("error = %q, want it to contain %q", err.Error(), c.want)
			}
		})
	}
}

// With AllowAnyField set, a field that the schema does not declare is admitted
// and its type is unknown, so any scalar may be compared against it. Fields the
// schema does declare keep their types, and are still checked.
func TestCondAllowAnyField(t *testing.T) {
	permissive := &CondSchema{
		Fields:         map[string]FieldKind{"holdings_count": FieldNumber},
		AllowAnyField:  true,
		AllowAnyFilter: true,
	}

	accepted := map[string]string{
		`{"type":"term","field":"undeclared","rel":"eq","value":"x"}`:       `undeclared = 'x'`,
		`{"type":"term","field":"undeclared","rel":"ge","value":3}`:         `undeclared >= 3`,
		`{"type":"term","field":"undeclared","rel":"eq","value":true}`:      `undeclared = true`,
		`{"type":"term","field":"undeclared","rel":"contains","value":"x"}`: `undeclared ilike '%x%'`,
		`{"type":"filter","name":"undeclared"}`:                             `filter(undeclared)`,
		// A declared field keeps the type it was declared with.
		`{"type":"term","field":"holdings_count","rel":"ge","value":3}`: `holdings_count >= 3`,
	}
	for doc, want := range accepted {
		got, err := ParseCond([]byte(doc), permissive)
		if err != nil {
			t.Errorf("%s: unexpectedly rejected: %v", doc, err)
		} else if got != want {
			t.Errorf("%s:\n got %q\nwant %q", doc, got, want)
		}
	}

	// Being permissive about names is not being permissive about syntax: a
	// field name that is not an identifier is still refused.
	rejected := map[string]string{
		`{"type":"term","field":"a; drop set x","rel":"eq","value":"x"}`:        `invalid field identifier`,
		`{"type":"term","field":"holdings_count","rel":"eq","value":"three"}`:   `needs a numeric value`,
		`{"type":"term","field":"holdings_count","rel":"contains","value":"x"}`: `cannot be matched with relation`,
	}
	for doc, want := range rejected {
		_, err := ParseCond([]byte(doc), permissive)
		if err == nil {
			t.Errorf("%s: unexpectedly accepted", doc)
		} else if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %q, want it to contain %q", doc, err, want)
		}
	}

	// Without the flag, an undeclared field is not queryable at all.
	strict := &CondSchema{Fields: map[string]FieldKind{"holdings_count": FieldNumber}}
	_, err := ParseCond([]byte(`{"type":"term","field":"undeclared","rel":"eq","value":"x"}`), strict)
	if err == nil {
		t.Error("an undeclared field was accepted without AllowAnyField")
	} else if want := `field is not queryable: "undeclared"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}

	if got, want := FieldAny.String(), "any"; got != want {
		t.Errorf("FieldAny.String() = %q, want %q", got, want)
	}
}

func readCase(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
