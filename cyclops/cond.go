package cyclops

// Decoding of structured search conditions.
//
// A client sends a condition as the JSON tree described by ramls/cond-schema.json
// rather than as a string of CCMS command language. The client therefore never
// authors CCMS syntax: it names a field, an abstract relation and a value, and
// the code here decides how that is spelled. Every identifier is checked against
// the fields the caller declares queryable, and every value is rendered through
// the sanitisation helpers in handlers.go, so a hostile value can only ever end
// up inside a correctly quoted literal.
//
//
// The translation to a CCMS condition is in two stages. DecodeCond turns bytes
// into a tree, checking only what is intrinsic to the format; RenderCond checks
// the tree against a caller-supplied schema and produces the CCMS condition.
// Keeping them apart means the tree can be inspected, logged or rewritten in
// between, and that rendering is generative: no part of the client's input is
// ever copied into the output except as a quoted literal or an allow-listed name.

import "bytes"
import "encoding/json"
import "fmt"
import "regexp"
import "strings"

// Limits on the size of a condition tree.
const maxCondDepth = 20
const maxCondNodes = 200
const maxCondListValues = 100

// FieldKind is the type of a queryable field, used to check that a value is
// of a form the field can meaningfully be compared against.
// "Kind" rather than "Type" because nodes have a type.
// (How does Go not have enums in 2026?)
type FieldKind int

const (
	FieldString FieldKind = iota
	FieldNumber
	FieldBoolean
	FieldDate
)

// String names the kind for use in error messages. An unrecognised kind is
// rendered in the conventional stringer form rather than as a plausible name,
// so that a bad value cannot pass for a real one.
func (k FieldKind) String() string {
	switch k {
	case FieldString:
		return "string"
	case FieldNumber:
		return "numeric"
	case FieldBoolean:
		return "boolean"
	case FieldDate:
		return "date"
	default:
		return fmt.Sprintf("FieldKind(%d)", int(k))
	}
}

// CondSchema declares what a condition is allowed to mention. It is the
// authorisation boundary: a field absent from Fields cannot be queried at all,
// which keeps a syntactically valid condition from reading data the client has
// no business seeing.
type CondSchema struct {
	// Fields maps queryable field names to their types.
	Fields map[string]FieldKind

	// Filters is the set of filter names that may be referenced.
	Filters map[string]bool

	// AllowAnyField and AllowAnyFilter relax the two allow-lists to "any
	// syntactically valid identifier". They exist for callers that do not
	// yet have a catalogue of fields to hand, and weaken the guarantee
	// above to injection-safety alone.
	AllowAnyField  bool
	AllowAnyFilter bool
}

// Clause is one node of a condition tree: a Junction, a Negation, a Term or a
// FilterRef. Those types are all implementations of this interface: see their
// definitions of the render() function below.
type Clause interface {
	render(s *CondSchema, b *strings.Builder) error
}

// Junction is a conjunction or disjunction of subordinate clauses.
type Junction struct {
	Op      string // "and" or "or"
	Clauses []Clause
}

// Negation is the negation of a single subordinate clause.
type Negation struct {
	Clause Clause
}

// Term compares a single field against a value. Rel is an abstract relation
// name, not a CCMS operator: the mapping to CCMS is made during rendering.
type Term struct {
	Field string
	Rel   string
	Value any // string, json.Number, bool, []any of those, or nil
}

// FilterRef refers to a named filter by name.
type FilterRef struct {
	Name string
}

// The operators that combine subordinate clauses, and the CCMS keyword each
// becomes.
var junctionOps = map[string]string{
	"and": "and",
	"or":  "or",
}

// Relations that compare a field against a single scalar, and the CCMS
// operator each becomes.
var scalarRels = map[string]string{
	"eq": "=",
	"ne": "<>",
	"lt": "<",
	"le": "<=",
	"gt": ">",
	"ge": ">=",
}

// Relations that match a substring of a string field, and the printf-like pattern
// each wraps the value in. The value itself is escaped by likePattern first.
var patternRels = map[string]string{
	"contains":   "%%%s%%",
	"startsWith": "%s%%",
	"endsWith":   "%%%s",
}

// Relations that compare a field against a list of scalars.
var listRels = map[string]string{
	"in":    "in",
	"notIn": "not in",
}

// Relations that take no value at all.
var nullRels = map[string]string{
	"isNull":    "is null",
	"isNotNull": "is not null",
}

// ParseCond decodes a JSON condition and renders it as a CCMS condition in a
// single step. It is the form callers normally want.
func ParseCond(data []byte, s *CondSchema) (string, error) {
	c, err := DecodeCond(data)
	if err != nil {
		return "", err
	}
	return RenderCond(c, s)
}

// DecodeCond decodes a condition tree from JSON, checking its shape but not
// yet the names it mentions.
func DecodeCond(data []byte) (Clause, error) {
	d := &condDecoder{}
	return d.clause(data, "condition", 0)
}

// RenderCond checks a decoded condition against a schema and renders it as a
// CCMS condition.
func RenderCond(c Clause, s *CondSchema) (string, error) {
	if c == nil {
		return "", fmt.Errorf("condition is empty")
	}
	if s == nil {
		return "", fmt.Errorf("no condition schema supplied")
	}
	var b strings.Builder
	err := c.render(s, &b)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// -----------------------------------------------------------------------------
// Decoding

type condDecoder struct {
	nodes int
}

// The four node shapes, each decoded strictly so that a property belonging to
// another shape -- "clauses" on a "not", say -- is an error rather than being
// quietly dropped.
type rawJunction struct {
	Type    string            `json:"type"`
	Clauses []json.RawMessage `json:"clauses"`
}

type rawNegation struct {
	Type   string          `json:"type"`
	Clause json.RawMessage `json:"clause"`
}

type rawTerm struct {
	Type  string          `json:"type"`
	Field string          `json:"field"`
	Rel   string          `json:"rel"`
	Value json.RawMessage `json:"value"`
}

type rawFilter struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// clause decodes one node, dispatching on its "type" discriminator. path names
// the node's position in the tree so that errors can point at it.
func (d *condDecoder) clause(data []byte, path string, depth int) (Clause, error) {
	if depth > maxCondDepth {
		return nil, fmt.Errorf("%s: condition nested more than %d deep", path, maxCondDepth)
	}
	d.nodes++
	if d.nodes > maxCondNodes {
		return nil, fmt.Errorf("condition has more than %d clauses", maxCondNodes)
	}

	var disc struct {
		Type string `json:"type"`
	}
	err := json.Unmarshal(data, &disc)
	if err != nil {
		return nil, fmt.Errorf("%s: not a condition clause: %v", path, err)
	}

	switch {
	case junctionOps[disc.Type] != "":
		return d.junction(data, path, depth)
	case disc.Type == "not":
		return d.negation(data, path, depth)
	case disc.Type == "term":
		return d.term(data, path)
	case disc.Type == "filter":
		return d.filter(data, path)
	case disc.Type == "":
		return nil, fmt.Errorf(`%s: clause has no "type"`, path)
	default:
		return nil, fmt.Errorf("%s: unknown clause type %q", path, disc.Type)
	}
}

func (d *condDecoder) junction(data []byte, path string, depth int) (Clause, error) {
	var raw rawJunction
	err := strictUnmarshal(data, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if len(raw.Clauses) == 0 {
		return nil, fmt.Errorf(`%s: %q clause has no subordinate clauses`, path, raw.Type)
	}
	j := &Junction{Op: raw.Type, Clauses: make([]Clause, len(raw.Clauses))}
	for i, sub := range raw.Clauses {
		c, err := d.clause(sub, fmt.Sprintf("%s.clauses[%d]", path, i), depth+1)
		if err != nil {
			return nil, err
		}
		j.Clauses[i] = c
	}
	return j, nil
}

func (d *condDecoder) negation(data []byte, path string, depth int) (Clause, error) {
	var raw rawNegation
	err := strictUnmarshal(data, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if len(raw.Clause) == 0 {
		return nil, fmt.Errorf(`%s: "not" clause has no subordinate clause`, path)
	}
	c, err := d.clause(raw.Clause, path+".clause", depth+1)
	if err != nil {
		return nil, err
	}
	return &Negation{Clause: c}, nil
}

func (d *condDecoder) term(data []byte, path string) (Clause, error) {
	var raw rawTerm
	err := strictUnmarshal(data, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if raw.Field == "" {
		return nil, fmt.Errorf(`%s: term has no "field"`, path)
	}
	if raw.Rel == "" {
		return nil, fmt.Errorf(`%s: term has no "rel"`, path)
	}

	t := &Term{Field: raw.Field, Rel: raw.Rel}

	switch {
	case nullRels[raw.Rel] != "":
		if len(raw.Value) > 0 {
			return nil, fmt.Errorf("%s: relation %q takes no value", path, raw.Rel)
		}
		return t, nil

	case listRels[raw.Rel] != "":
		var list []any
		err = decodeValue(raw.Value, &list, path)
		if err != nil {
			return nil, fmt.Errorf("%s: relation %q needs a list of values: %v", path, raw.Rel, err)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("%s: relation %q needs a non-empty list of values", path, raw.Rel)
		}
		if len(list) > maxCondListValues {
			return nil, fmt.Errorf("%s: relation %q has more than %d values", path, raw.Rel, maxCondListValues)
		}
		for i, v := range list {
			if !isScalar(v) {
				return nil, fmt.Errorf("%s.value[%d]: not a string, number or boolean", path, i)
			}
		}
		t.Value = list
		return t, nil

	case scalarRels[raw.Rel] != "" || patternRels[raw.Rel] != "":
		var v any
		err = decodeValue(raw.Value, &v, path)
		if err != nil {
			return nil, fmt.Errorf("%s: relation %q needs a value: %v", path, raw.Rel, err)
		}
		if !isScalar(v) {
			return nil, fmt.Errorf("%s.value: not a string, number or boolean", path)
		}
		t.Value = v
		return t, nil

	default:
		return nil, fmt.Errorf("%s: unknown relation %q", path, raw.Rel)
	}
}

func (d *condDecoder) filter(data []byte, path string) (Clause, error) {
	var raw rawFilter
	err := strictUnmarshal(data, &raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if raw.Name == "" {
		return nil, fmt.Errorf(`%s: filter reference has no "name"`, path)
	}
	return &FilterRef{Name: raw.Name}, nil
}

// strictUnmarshal decodes exactly one JSON value into v, rejecting properties v
// does not declare, and preserving numbers as json.Number so that a value's
// precision survives the round trip.
func strictUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	err := dec.Decode(v)
	if err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected trailing data after clause")
	}
	return nil
}

// decodeValue decodes a term's value, treating an absent value and an explicit
// null alike: neither is a value, and a null field is expressed with the
// "isNull" relation instead.
func decodeValue(data json.RawMessage, v any, path string) error {
	if len(data) == 0 {
		return fmt.Errorf("no value given")
	}
	if string(data) == "null" {
		return fmt.Errorf(`value is null (use the "isNull" relation)`)
	}
	return strictUnmarshal(data, v)
}

func isScalar(v any) bool {
	switch v.(type) {
	case string, json.Number, bool:
		return true
	}
	return false
}

// -----------------------------------------------------------------------------
// Rendering

func (j *Junction) render(s *CondSchema, b *strings.Builder) error {
	op := junctionOps[j.Op]
	if op == "" {
		return fmt.Errorf("unknown junction operator %q", j.Op)
	}
	b.WriteByte('(')
	for i, c := range j.Clauses {
		if i > 0 {
			b.WriteString(" " + op + " ")
		}
		err := c.render(s, b)
		if err != nil {
			return err
		}
	}
	b.WriteByte(')')
	return nil
}

func (n *Negation) render(s *CondSchema, b *strings.Builder) error {
	b.WriteString("not ")
	_, isJunction := n.Clause.(*Junction)
	if !isJunction {
		b.WriteByte('(')
		defer b.WriteByte(')')
	}
	return n.Clause.render(s, b)
}

func (f *FilterRef) render(s *CondSchema, b *strings.Builder) error {
	if !s.AllowAnyFilter && !s.Filters[f.Name] {
		return fmt.Errorf("unknown filter: %q", f.Name)
	}
	name, err := ident("filter", f.Name)
	if err != nil {
		return err
	}
	b.WriteString("filter(" + name + ")")
	return nil
}

func (t *Term) render(s *CondSchema, b *strings.Builder) error {
	kind, err := t.fieldKind(s)
	if err != nil {
		return err
	}
	field, err := ident("field", t.Field)
	if err != nil {
		return err
	}

	if op := nullRels[t.Rel]; op != "" {
		b.WriteString(field + " " + op)
		return nil
	}

	if pattern := patternRels[t.Rel]; pattern != "" {
		return t.renderPattern(pattern, field, kind, b)
	}

	if op := listRels[t.Rel]; op != "" {
		return t.renderList(op, field, kind, b)
	}

	op := scalarRels[t.Rel]
	if op == "" {
		return fmt.Errorf("unknown relation %q", t.Rel)
	}
	lit, err := renderLiteral(t.Value, kind, t.Field)
	if err != nil {
		return err
	}
	b.WriteString(field + " " + op + " " + lit)
	return nil
}

// renderPattern renders a substring match. The value is escaped as a LIKE
// pattern, wrapped in whichever wildcards the relation calls for, and quoted.
func (t *Term) renderPattern(pattern, field string, kind FieldKind, b *strings.Builder) error {
	if kind != FieldString {
		return fmt.Errorf("field %q cannot be matched with relation %q", t.Field, t.Rel)
	}
	str, ok := t.Value.(string)
	if !ok {
		return fmt.Errorf("relation %q needs a string value", t.Rel)
	}
	lit, err := sqlString(fmt.Sprintf(pattern, likePattern(str)))
	if err != nil {
		return err
	}
	b.WriteString(field + " ilike " + lit)
	return nil
}

// renderList renders a membership test against a parenthesised list of literals.
func (t *Term) renderList(op, field string, kind FieldKind, b *strings.Builder) error {
	list, ok := t.Value.([]any)
	if !ok {
		return fmt.Errorf("relation %q needs a list of values", t.Rel)
	}
	lits := make([]string, len(list))
	for i, v := range list {
		lit, err := renderLiteral(v, kind, t.Field)
		if err != nil {
			return err
		}
		lits[i] = lit
	}
	b.WriteString(field + " " + op + " (" + strings.Join(lits, ", ") + ")")
	return nil
}

// fieldKind resolves the declared type of the term's field, which is also the
// check that the field may be queried at all. An undeclared field is admitted
// only when AllowAnyField is set, and is then treated as a string, that being
// the zero value of FieldKind.
func (t *Term) fieldKind(s *CondSchema) (FieldKind, error) {
	kind, ok := s.Fields[t.Field]
	if !s.AllowAnyField && !ok {
		return FieldString, fmt.Errorf("field is not queryable: %q", t.Field)
	}
	return kind, nil
}

// renderLiteral renders a scalar as a CCMS literal of the kind the field expects.
func renderLiteral(v any, kind FieldKind, field string) (string, error) {
	switch val := v.(type) {
	case string:
		if kind != FieldString && kind != FieldDate {
			return "", fmt.Errorf("field %q needs a %s value, not a string", field, kind)
		}
		return sqlString(val)
	case json.Number:
		if kind != FieldNumber {
			return "", fmt.Errorf("field %q needs a %s value, not a number", field, kind)
		}
		return renderNumber(val)
	case bool:
		if kind != FieldBoolean {
			return "", fmt.Errorf("field %q needs a %s value, not a boolean", field, kind)
		}
		if val {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("field %q: unsupported value type %T", field, v)
	}
}

// decimalRe matches a plain decimal number, the only non-integer numeric form
// with a counterpart in the CCMS grammar.
var decimalRe = regexp.MustCompile(`^-?[0-9]+\.[0-9]+$`)

// renderNumber renders a JSON number as a CCMS numeric literal. Integers are
// checked by the same rule as every other integer in a command; anything else
// is rendered only if it is a plain decimal, since exponent notation has no
// counterpart in the grammar.
func renderNumber(n json.Number) (string, error) {
	s := n.String()
	v, err := intval(s)
	if err == nil {
		return v, nil
	}
	if decimalRe.MatchString(s) {
		return s, nil
	}
	return "", fmt.Errorf("invalid number: %q", s)
}

// likePattern escapes the wildcards of a LIKE pattern, so that a value
// containing '%' or '_' matches those characters literally rather than
// silently becoming a wildcard search.
func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
