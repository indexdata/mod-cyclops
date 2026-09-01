package cyclops

import "errors"
import "regexp"
import "strings"
import "strconv"
import "io"
import "fmt"
import "net/http"
import "encoding/json"
import "github.com/go-chi/chi/v5"
import "github.com/indexdata/ccms"

type TagList struct {
	Tags []any `json:"tags"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowTags(w http.ResponseWriter, req *http.Request, caption string) error {
	resp, err := server.sendToCCMS(caption, "show tags;")
	if err != nil {
		return err
	}

	result := readResults(resp)[0]
	tags := make([]any, 0)
	for val := range result.Data() {
		tags = append(tags, val.Values()[0])
	}
	tagList := TagList{Tags: tags}
	return server.respondWithJSON(w, tagList, caption)
}

// -----------------------------------------------------------------------------

type DefineTag struct {
	Name string `json:"name"`
}

func (server *ModCyclopsServer) handleDefineTag(w http.ResponseWriter, req *http.Request, caption string) error {
	var tag DefineTag
	err := unmarshalBody(req, &tag)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	name, err := ident("tag", tag.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "define tag " + name
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+tag.Name, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

// fieldIndex maps each of the named fields to the column it occupies in the
// result, so that handlers need not rely on CCMS returning them in any
// particular order. It fails unless all the named fields are present.
func fieldIndex(result ccms.Result, names ...string) (map[string]int, error) {
	index := make(map[string]int, len(result.Fields()))
	for i, field := range result.Fields() {
		index[field.Name()] = i
	}
	for _, name := range names {
		if _, ok := index[name]; !ok {
			return nil, fmt.Errorf("no '%s' field in response", name)
		}
	}
	return index, nil
}

// -----------------------------------------------------------------------------

type FilterSummary struct {
	Project    string `json:"project"`
	Filter     string `json:"filter"`
	Definition string `json:"definition"`
}

type FilterList struct {
	Filters []FilterSummary `json:"filters"`
	// No other elements yet, but use a structure for future expansion
}

// handleShowFilters lists all known filters, or -- when the optional "project"
// query parameter is supplied -- only those in the named project.
func (server *ModCyclopsServer) handleShowFilters(w http.ResponseWriter, req *http.Request, caption string) error {
	command := "show filters;"
	project := req.URL.Query().Get("project")
	if project != "" {
		projectId, err := ident("project", project)
		if err != nil {
			return fmt.Errorf("%s: %w", caption, err)
		}
		command = "show filters in project " + projectId + ";"
		caption += " in project " + projectId
	}
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption, command)
	if err != nil {
		return err
	}

	result := readResults(resp)[0]
	index, err := fieldIndex(result, "project", "filter", "definition")
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	filters := make([]FilterSummary, 0)
	for val := range result.Data() {
		values := val.Values()
		filters = append(filters, FilterSummary{
			Project:    mustString(values[index["project"]]),
			Filter:     mustString(values[index["filter"]]),
			Definition: mustString(values[index["definition"]]),
		})
	}
	filterList := FilterList{Filters: filters}
	return server.respondWithJSON(w, filterList, caption)
}

// -----------------------------------------------------------------------------

type CreateFilter struct {
	Name     string `json:"name"`
	Cond     string `json:"cond"`
	Template string `json:"template"`
}

func (server *ModCyclopsServer) handleCreateFilter(w http.ResponseWriter, req *http.Request, caption string) error {
	var filter CreateFilter
	err := unmarshalBody(req, &filter)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	name, err := ident("filter", filter.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "create filter " + name
	if filter.Cond != "" {
		// XXX injection risk: 'cond' is a free-form condition expression and is
		// not sanitised; needs AST-based construction.
		command += " where " + filter.Cond
	}
	if filter.Template != "" {
		// XXX injection risk: 'template' is a free-form expression and is not
		// sanitised; needs AST-based construction.
		command += " template " + filter.Template
	}
	command += ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+filter.Name, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleDeleteFilter(w http.ResponseWriter, req *http.Request, caption string) error {
	// Filters are namespaced to their project, so the identifier here is the
	// qualified "project.filter" that the filter was created under: `ident`
	// admits the '.' that joins the two parts.
	filterId, err := ident("filter", chi.URLParam(req, "filterId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "drop filter " + filterId + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+filterId, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type SetSummary struct {
	Project string `json:"project"`
	Set     string `json:"set"`
	Title   string `json:"title"`
}

type SetList struct {
	Sets []SetSummary `json:"sets"`
	// No other elements yet, but use a structure for future expansion
}

// readSetList translates the three-column response that CCMS returns for the
// "show sets" commands, keying the columns by name rather than by position.
func readSetList(result ccms.Result) (SetList, error) {
	index, err := fieldIndex(result, "project", "set", "title")
	if err != nil {
		return SetList{}, err
	}

	sets := make([]SetSummary, 0)
	for val := range result.Data() {
		values := val.Values()
		sets = append(sets, SetSummary{
			Project: mustString(values[index["project"]]),
			Set:     mustString(values[index["set"]]),
			Title:   mustString(values[index["title"]]),
		})
	}
	return SetList{Sets: sets}, nil
}

func (server *ModCyclopsServer) handleShowSets(w http.ResponseWriter, req *http.Request, caption string) error {
	resp, err := server.sendToCCMS(caption, "show sets;")
	if err != nil {
		return fmt.Errorf("could not fetch show-sets response: %w", err)
	}

	setList, err := readSetList(readResults(resp)[0])
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	return server.respondWithJSON(w, setList, caption)
}

// -----------------------------------------------------------------------------

type CreateSet struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}

// As with projects, CCMS's "create set" facility only brings the set
// into existence, so a client-supplied title has to be applied by a
// second command, "alter set".
func (server *ModCyclopsServer) handleCreateSet(w http.ResponseWriter, req *http.Request, caption string) error {
	var set CreateSet
	err := unmarshalBody(req, &set)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	name, err := ident("set", set.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "create set " + name + ";"
	if set.Title != "" {
		title, err2 := sqlString(set.Title)
		if err2 != nil {
			return fmt.Errorf("%s: %w", caption, err2)
		}
		command += "\nalter set " + name + " alter property title set " + title + ";"
	}
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+set.Name, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

// AlterSet carries the new title for an existing set. The set's name comes
// from the URL, so a "name" in the body -- which set-schema.json allows, since
// the same schema describes set creation -- is ignored.
type AlterSet = CreateSet

func (server *ModCyclopsServer) handleAlterSet(w http.ResponseWriter, req *http.Request, caption string) error {
	name, err := ident("set", chi.URLParam(req, "setName"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	var set AlterSet
	err = unmarshalBody(req, &set)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	title, err := sqlString(set.Title)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "alter set " + name + " alter property title set " + title + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+name, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func makeConditionalClause(cond, filter, tag, omitTag, sort, limit, offset string) (string, error) {
	var b strings.Builder

	if cond != "" {
		b.WriteString(" where ")
		// XXX injection risk, but only by way of the 'cond' query parameter,
		// which is interpolated unchanged. A condition arriving as 'jsonCond'
		// has been built by ParseCond from a validated structure and is safe.
		// The risk goes away when 'cond' is withdrawn.
		b.WriteString(cond)
	}

	if filter != "" {
		v, err := ident("filter", filter)
		if err != nil {
			return "", err
		}
		b.WriteString(" filter ")
		b.WriteString(v)
	}

	if tag != "" && omitTag != "" {
		return "", errors.New("both 'tag' and 'omitTag' parameters supplied")
	}

	if tag != "" {
		v, err := ident("tag", tag)
		if err != nil {
			return "", err
		}
		b.WriteString(" tag ")
		b.WriteString(v)
	} else if omitTag != "" {
		v, err := ident("omitTag", omitTag)
		if err != nil {
			return "", err
		}
		b.WriteString(" tag not ")
		b.WriteString(v)
	}

	if sort != "" {
		v, err := sortList(sort)
		if err != nil {
			return "", err
		}
		b.WriteString(" order by ")
		b.WriteString(v)
	}

	if limit != "*" {
		if limit == "" {
			limit = "100"
		}
		v, err := intval(limit)
		if err != nil {
			return "", err
		}
		b.WriteString(" limit ")
		b.WriteString(v)
	}

	if offset != "" {
		v, err := intval(offset)
		if err != nil {
			return "", err
		}
		b.WriteString(" offset ")
		b.WriteString(v)
	}

	return b.String(), nil
}

func makeSelectClause(fields, setName, cond, filter, tag, omitTag, sort, limit, offset string) (string, error) {
	if fields == "" {
		return "", errors.New("no 'fields' parameter supplied")
	}

	validFields, err := fieldList(fields)
	if err != nil {
		return "", err
	}

	validSet, err := ident("set", setName)
	if err != nil {
		return "", err
	}

	conditionalClause, err := makeConditionalClause(cond, filter, tag, omitTag, sort, limit, offset)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	b.WriteString("select ")
	b.WriteString(validFields)

	b.WriteString(" from ")
	b.WriteString(validSet)

	b.WriteString(conditionalClause)
	return b.String(), nil
}

// getCondSchema provides the schema against which a 'jsonCond' parameter is
// validated. Until mod-cyclops knows which fields each project exposes, any
// syntactically valid field and filter name is admitted, so what this buys is
// injection-safety rather than authorisation: see the commentary in cond.go.
func getCondSchema() *CondSchema {
	return &CondSchema{AllowAnyField: true, AllowAnyFilter: true}
}

// requestCond returns the WHERE condition for a retrieval, which the caller may
// supply either as 'cond', a condition already in CCMS's own language, or as
// 'jsonCond', the structured form described by ramls/cond-schema.json. The two
// are alternatives: supplying both is an error, and supplying neither means the
// retrieval is unconditional, as it always has been.
//
// 'cond' is interpolated into the command unchanged and is therefore an
// injection risk; 'jsonCond' is validated and rendered by mod-cyclops itself.
// The intention is to withdraw 'cond' once clients have moved over.
func requestCond(req *http.Request) (string, error) {
	cond := req.URL.Query().Get("cond")
	jsonCond := req.URL.Query().Get("jsonCond")

	if cond != "" && jsonCond != "" {
		return "", &HTTPError{
			status:  http.StatusBadRequest,
			message: "only one of 'cond' and 'jsonCond' may be supplied",
		}
	}
	if jsonCond == "" {
		return cond, nil
	}

	rendered, err := ParseCond([]byte(jsonCond), getCondSchema())
	if err != nil {
		return "", &HTTPError{
			status:  http.StatusBadRequest,
			message: fmt.Sprintf("invalid 'jsonCond' parameter: %s", err),
		}
	}
	return rendered, nil
}

func makeRetrieveCommand(req *http.Request, countOnly bool) (string, error) {
	selectFields := req.URL.Query().Get("fields")
	if countOnly {
		selectFields = "COUNT(*)"
	}

	cond, err := requestCond(req)
	if err != nil {
		return "", err
	}

	selectClause, err := makeSelectClause(
		selectFields,
		chi.URLParam(req, "setName"),
		cond,
		req.URL.Query().Get("filter"),
		req.URL.Query().Get("tag"),
		req.URL.Query().Get("omitTag"),
		req.URL.Query().Get("sort"),
		req.URL.Query().Get("limit"),
		req.URL.Query().Get("offset"),
	)
	if err != nil {
		return "", err
	}
	return selectClause + ";", nil
}

// Specify the JSON encoding.

type FieldDescription struct {
	Name string `json:"name"`
	// No other elements yet, but use a structure for future expansion
}

type DataRow struct {
	Values []any `json:"values"`
	// No other elements yet, but use a structure for future expansion
}

type RetrieveResponse struct {
	Status  string             `json:"status"`
	Fields  []FieldDescription `json:"fields"`
	Data    []DataRow          `json:"data"`
	Message string             `json:"message"`
}

// Translate from CCMS's API into structures with JSON encoding instructions
func ccms2local(rr *ccms.Response) RetrieveResponse {
	r := readResults(rr)[0]
	localFields := make([]FieldDescription, len(r.Fields()))
	for i, val := range r.Fields() {
		localFields[i].Name = val.Name()
	}

	localData := make([]DataRow, 0)
	for val := range r.Data() {
		values := val.Values()
		row := DataRow{Values: make([]any, len(values))}
		copy(row.Values, values)
		localData = append(localData, row)
	}

	return RetrieveResponse{
		Status:  r.Status(),
		Fields:  localFields,
		Data:    localData,
		Message: r.Message(),
	}
}

func (server *ModCyclopsServer) handleRetrieve(w http.ResponseWriter, req *http.Request, caption string) error {
	coString := req.URL.Query().Get("countOnly")
	if coString == "" {
		coString = "false"
	}
	countOnly, err := strconv.ParseBool(coString)
	if err != nil {
		return fmt.Errorf("could not parse boolean 'countOnly' parameter: %w", err)
	}

	command, err := makeRetrieveCommand(req, countOnly)
	if err != nil {
		return fmt.Errorf("could not make retrieve command: %w", err)
	}
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption+" "+chi.URLParam(req, "setName"), command)
	if err != nil {
		return fmt.Errorf("could not retrieve: %w", err)
	}

	localrr := ccms2local(resp)
	return server.respondWithJSON(w, localrr, caption)
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleDropSet(w http.ResponseWriter, req *http.Request, caption string) error {
	setName, err := ident("set", chi.URLParam(req, "setName"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "drop set " + setName + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+setName, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type AddRecords struct {
	From    string `json:"from"`
	Cond    string `json:"cond"`
	Filter  string `json:"filter"`
	Tag     string `json:"tag"`
	OmitTag string `json:"omitTag"`
	Limit   string `json:"limit"`
}

func (server *ModCyclopsServer) handleAddObjects(w http.ResponseWriter, req *http.Request, caption string) error {
	setName, err := ident("set", chi.URLParam(req, "setName"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	var params AddRecords
	err = unmarshalBody(req, &params)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	limit := params.Limit
	if limit == "" {
		limit = "*" // Omit "limit" from the command when the request did not specify one
	}
	clause, err := makeSelectClause(
		"*",
		params.From,
		params.Cond,
		params.Filter,
		params.Tag,
		params.OmitTag,
		"",    // Sort
		limit, // "*" omits "limit" completely when none was requested
		"",    // Offset
	)
	if err != nil {
		return fmt.Errorf("could not make select clause: %w", err)
	}
	command := "insert into " + setName + " " + clause + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+setName, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type RemoveRecords struct {
	Cond    string `json:"cond"`
	Filter  string `json:"filter"`
	Tag     string `json:"tag"`
	OmitTag string `json:"omitTag"`
	Limit   string `json:"limit"`
}

func (server *ModCyclopsServer) handleRemoveObjects(w http.ResponseWriter, req *http.Request, caption string) error {
	setName, err := ident("set", chi.URLParam(req, "setName"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	var params RemoveRecords
	err = unmarshalBody(req, &params)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	clause, err := makeConditionalClause(
		params.Cond,
		params.Filter,
		params.Tag,
		params.OmitTag,
		"",  // Sort
		"*", // Special-case value to omit "limit" completely
		"",  // Offset
	)
	if err != nil {
		return fmt.Errorf("could not make conditional clause: %w", err)
	}
	command := "delete from " + setName + " " + clause + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+setName, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type UpdateRecord struct {
	Decision bool   `json:"decision"`
	Fund     string `json:"fund"`
}

func (server *ModCyclopsServer) handleUpdateRecord(w http.ResponseWriter, req *http.Request, caption string) error {
	setName := chi.URLParam(req, "setName")
	recordId := chi.URLParam(req, "recordId")

	var record UpdateRecord
	err := unmarshalBody(req, &record)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	// A qualified set name such as "foo.bar" refers to the objects within the
	// set, which CCMS addresses as "foo.object".
	prefix, _, found := strings.Cut(setName, ".")
	if found {
		setName = prefix + ".object"
	}

	validSet, err := ident("set", setName)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	validId, err := intval(recordId)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	validFund, err := ident("fund", record.Fund)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := fmt.Sprintf("update %s set decision = %v, fund = %s where id = %s;",
		validSet, record.Decision, validFund, validId)
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+setName+"/"+recordId, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type BatchChanges struct {
	Decision *bool   `json:"decision"`
	Fund     *string `json:"fund"`
}

type BatchUpdate struct {
	Ids     []string     `json:"ids"`
	Changes BatchChanges `json:"changes"`
}

// handleBatchUpdate applies the same field-changes to many records at once. It
// works like handleUpdateRecord, but rather than addressing a single record by
// its URL-supplied id it runs a single "update" command whose "where id IN
// (...)" clause names every id listed in the request body.
func (server *ModCyclopsServer) handleBatchUpdate(w http.ResponseWriter, req *http.Request, caption string) error {
	setName := chi.URLParam(req, "setName")

	var batch BatchUpdate
	err := unmarshalBody(req, &batch)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	// A qualified set name such as "foo.bar" refers to the objects within the
	// set, which CCMS addresses as "foo.object".
	prefix, _, found := strings.Cut(setName, ".")
	if found {
		setName = prefix + ".object"
	}

	if len(batch.Ids) == 0 {
		return fmt.Errorf("%s: no ids specified", caption)
	}

	validSet, err := ident("set", setName)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	// Each id must be validated to avoid statement injection, exactly as the
	// single-record update validates its URL-supplied id.
	validIds := make([]string, len(batch.Ids))
	for i, id := range batch.Ids {
		validId, idErr := intval(id)
		if idErr != nil {
			return fmt.Errorf("%s: %w", caption, idErr)
		}
		validIds[i] = validId
	}

	var assignments []string
	if batch.Changes.Decision != nil {
		assignments = append(assignments,
			fmt.Sprintf("decision = %v", *batch.Changes.Decision))
	}
	if batch.Changes.Fund != nil {
		validFund, fundErr := ident("fund", *batch.Changes.Fund)
		if fundErr != nil {
			return fmt.Errorf("%s: %w", caption, fundErr)
		}
		assignments = append(assignments,
			fmt.Sprintf("fund = %s", validFund))
	}
	if len(assignments) == 0 {
		return fmt.Errorf("%s: no changes specified", caption)
	}
	command := fmt.Sprintf("update %s set %s where id IN (%s);",
		validSet, strings.Join(assignments, ", "), strings.Join(validIds, ", "))
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+setName, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleAddRemoveTags(w http.ResponseWriter, req *http.Request, caption string) error {
	// It seems weird to just shrug and say "fine" for anything posted, but for now it will suffice.
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

type BriefProject struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	// More to come, surely
}

type ProjectList struct {
	Projects []BriefProject `json:"projects"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowProjects(w http.ResponseWriter, req *http.Request, caption string) error {
	resp, err := server.sendToCCMS(caption, "show projects;")
	if err != nil {
		return err
	}

	result := readResults(resp)[0]
	projects := make([]BriefProject, 0)
	for val := range result.Data() {
		values := val.Values()
		bf := BriefProject{
			Id:   mustString(values[0]),
			Name: mustString(values[1]),
		}
		projects = append(projects, bf)
	}

	projectList := ProjectList{Projects: projects}
	return server.respondWithJSON(w, projectList, caption)
}

// -----------------------------------------------------------------------------

type ProjectItem struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ProjectAction struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ProjectFund = ProjectItem

type ProjectPerson struct {
	XId  string `json:"xid"`
	Role string `json:"role"`
}

type ProjectLocation = ProjectItem

type ProjectTrack = ProjectItem

type Project struct {
	Id           string            `json:"id"`
	Name         string            `json:"name"`
	Action       ProjectAction     `json:"action"`
	MouLink      string            `json:"mou_link"`
	Funds        []ProjectFund     `json:"funds"`
	People       []ProjectPerson   `json:"people"`
	Origins      []ProjectLocation `json:"origins"`
	Destinations []ProjectLocation `json:"destinations"`
	Tracks       []ProjectTrack    `json:"tracks"`
}

// The representation of list-valued fields when retrieving a project
// from CCMS is as a single text-field that is a pipe-separated list
// of colon-separated id:description pairs. It is of the form
//
//	<slug1>:<desc1>|<slug2>:<desc2>
//
// e.g.
//
//	coalition_slavic_lit:Coalition for Slavic literature|palci_cultural:PALCI cultural preservation
//
// -
func string2array(s string) []ProjectItem {
	parts := strings.Split(s, "|")
	if len(parts) == 1 && parts[0] == "" {
		return []ProjectItem{}
	}

	items := make([]ProjectItem, len(parts))
	for i, segment := range parts {
		items[i] = string2item(segment)
	}
	return items
}

// string2item parses a single <slug>:<description> pair. A newly created
// project has empty values for fields that have not yet been set, and such a
// value has no colon in it at all, so the description is simply omitted.
func string2item(s string) ProjectItem {
	id, name, _ := strings.Cut(s, ":")
	return ProjectItem{Id: id, Name: name}
}

func (server *ModCyclopsServer) fetchProject(caption string, projectId string) (Project, error) {
	command := "show project " + projectId + ";"
	server.Log("command", command)
	resp, err := server.sendToCCMS(caption, command)
	if err != nil {
		return Project{}, err
	}

	result := readResults(resp)[0]
	project := Project{
		Id: projectId,
	}

	for val := range result.Data() {
		pair := val.Values()
		key := mustString(pair[0])
		value := pair[1]

		switch key {
		case "title":
			project.Name = mustString(value)
		case "action":
			project.Action = ProjectAction(string2item(mustString(value)))
		case "mou_link":
			project.MouLink = mustString(value)
		case "funds":
			project.Funds = string2array(mustString(value))
		case "origins":
			project.Origins = string2array(mustString(value))
		case "destinations":
			project.Destinations = string2array(mustString(value))
		default:
			server.Log("data", "unrecognised Project field", key, "=", fmt.Sprintf("%+v", value))
		}
	}

	return project, nil
}

func (server *ModCyclopsServer) handleFetchProject(w http.ResponseWriter, req *http.Request, caption string) error {
	projectId, err := ident("project", chi.URLParam(req, "projectId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	project, err := server.fetchProject(caption, projectId)
	if err != nil {
		return err
	}

	return server.respondWithJSON(w, project, caption)
}

// -----------------------------------------------------------------------------

// CCMS's "create project" facility literally only creates the
// project, but doesn't set any of its fields. So we need to make two
// calls: one to bring the empty project into existence, and once to
// set the specified values.
// -
func (server *ModCyclopsServer) handleCreateProject(w http.ResponseWriter, req *http.Request, caption string) error {
	var project Project
	err := unmarshalBody(req, &project)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	if project.Id == "" {
		return fmt.Errorf("%s: no id specified", caption)
	}
	id, err := ident("project", project.Id)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	// A freshly created project has no funds yet.
	body, err := project2command(id, project, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	command := "create project " + id + ";\n" + body
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+project.Id, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (server *ModCyclopsServer) handleDeleteProject(w http.ResponseWriter, req *http.Request, caption string) error {
	projectId, err := ident("project", chi.URLParam(req, "projectId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "drop project " + projectId + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+projectId, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func project2command(projectId string, project Project, existingFunds []ProjectFund) (string, error) {
	id, err := ident("project", projectId)
	if err != nil {
		return "", err
	}
	name, err := sqlString(project.Name)
	if err != nil {
		return "", err
	}
	action, err := ident("action", project.Action.Id)
	if err != nil {
		return "", err
	}
	mouLink, err := sqlString(project.MouLink)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("alter project " + id + " alter property title set " + name + ";\n")
	b.WriteString("alter project " + id + " alter property action set " + action + ";\n")
	b.WriteString("alter project " + id + " alter property mou_link set " + mouLink + ";\n")
	// Compute the minimal set of fund changes by comparing the
	// existing list of funds against the new one, rather than dropping
	// all and re-adding.
	oldFunds := make(map[string]bool)
	for _, fund := range existingFunds {
		fundId, err := ident("fund", fund.Id)
		if err != nil {
			return "", err
		}
		oldFunds[fundId] = true
	}
	newFunds := make(map[string]bool)
	for _, fund := range project.Funds {
		fundId, err := ident("fund", fund.Id)
		if err != nil {
			return "", err
		}
		newFunds[fundId] = true
	}
	for _, fund := range project.Funds {
		fundId, _ := ident("fund", fund.Id)
		if !oldFunds[fundId] {
			b.WriteString("alter project " + id + " alter property funds add " + fundId + ";\n")
		}
	}
	for _, fund := range existingFunds {
		fundId, _ := ident("fund", fund.Id)
		if !newFunds[fundId] {
			b.WriteString("alter project " + id + " alter property funds drop " + fundId + ";\n")
		}
	}
	// b.WriteString("alter project " + id + " alter property people set '" + project.People + "';\n")
	b.WriteString("alter project " + id + " alter property origins drop all;\n")
	for _, location := range project.Origins {
		locId, err := ident("origin", location.Id)
		if err != nil {
			return "", err
		}
		b.WriteString("alter project " + id + " alter property origins add " + locId + ";\n")
	}
	b.WriteString("alter project " + id + " alter property destinations drop all;\n")
	for _, location := range project.Destinations {
		locId, err := ident("destination", location.Id)
		if err != nil {
			return "", err
		}
		b.WriteString("alter project " + id + " alter property destinations add " + locId + ";\n")
	}
	// b.WriteString("alter project " + id + " alter property tracks set '" + project.Tracks + "'\n")
	return b.String(), nil
}

func (server *ModCyclopsServer) handleUpdateProject(w http.ResponseWriter, req *http.Request, caption string) error {
	projectId, err := ident("project", chi.URLParam(req, "projectId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	var project Project
	err = unmarshalBody(req, &project)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	// Fetch the current state of the project so that we can generate a
	// minimal set of fund changes rather than dropping all and re-adding.
	existing, err := server.fetchProject(caption, projectId)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command, err := project2command(projectId, project, existing.Funds)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+projectId, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (server *ModCyclopsServer) handleShowSetsInProject(w http.ResponseWriter, req *http.Request, caption string) error {
	projectId, err := ident("project", chi.URLParam(req, "projectId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	command := "show sets in project " + projectId + ";"
	server.Log("command", command)
	resp, err := server.sendToCCMS(caption+" "+projectId, command)
	if err != nil {
		return fmt.Errorf("could not fetch show-sets response: %w", err)
	}

	setList, err := readSetList(readResults(resp)[0])
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	return server.respondWithJSON(w, setList, caption)
}

// -----------------------------------------------------------------------------

type Fund struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	// More to come, surely
}

type FundList struct {
	Funds []Fund `json:"funds"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowFunds(w http.ResponseWriter, req *http.Request, caption string) error {
	resp, err := server.sendToCCMS(caption, "show funds;")
	if err != nil {
		return err
	}

	result := readResults(resp)[0]
	funds := make([]Fund, 0)
	for val := range result.Data() {
		values := val.Values()
		funds = append(funds, Fund{
			Id:   mustString(values[0]),
			Name: mustString(values[1]),
		})
	}
	fundList := FundList{Funds: funds}
	return server.respondWithJSON(w, fundList, caption)
}

// -----------------------------------------------------------------------------

type CreateFund struct {
	Id string `json:"id"`
}

func (server *ModCyclopsServer) handleCreateFund(w http.ResponseWriter, req *http.Request, caption string) error {
	var fund CreateFund
	err := unmarshalBody(req, &fund)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	id, err := ident("fund", fund.Id)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "create fund " + id + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+fund.Id, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) fetchFund(caption string, id string) (Fund, error) {
	command := "show fund " + id + ";"
	server.Log("command", command)
	resp, err := server.sendToCCMS(caption, command)
	if err != nil {
		return Fund{}, err
	}

	result := readResults(resp)[0]
	fund := Fund{
		Id: id,
	}

	for val := range result.Data() {
		pair := val.Values()
		key := mustString(pair[0])
		value := pair[1]

		switch key {
		case "title":
			fund.Name = mustString(value)
		default:
			server.Log("data", "unrecognised Fund field", key, "=", fmt.Sprintf("%+v", value))
		}
	}

	return fund, nil
}

func (server *ModCyclopsServer) handleFetchFund(w http.ResponseWriter, req *http.Request, caption string) error {
	id, err := ident("fund", chi.URLParam(req, "fundId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	fund, err := server.fetchFund(caption, id)
	if err != nil {
		return err
	}

	return server.respondWithJSON(w, fund, caption)
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleUpdateFund(w http.ResponseWriter, req *http.Request, caption string) error {
	id, err := ident("fund", chi.URLParam(req, "fundId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	var fund Fund
	err = unmarshalBody(req, &fund)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	name, err := sqlString(fund.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "alter fund " + id + " alter property title set " + name + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+id, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleDeleteFund(w http.ResponseWriter, req *http.Request, caption string) error {
	id, err := ident("fund", chi.URLParam(req, "fundId"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "drop fund " + id + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+id, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func unmarshalBody[T any](req *http.Request, data *T) error {
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("could not read HTTP request body: %w", err)
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return fmt.Errorf("could not deserialize JSON from body: %w", err)
	}

	return nil
}

func (server *ModCyclopsServer) sendToCCMS(caption string, command string) (*ccms.Response, error) {
	resp, err := server.ccmsClient.Send(command)
	if err != nil {
		return nil, fmt.Errorf("could not %s: %w", caption, err)
	}

	respString := respToString(resp)
	server.Log("ccms", respString)

	for _, result := range readResults(resp) {
		if result.Status() == "error" {
			return nil, fmt.Errorf("%s failed: %s", caption, result.Message())
		}
	}
	return resp, nil
}

func (server *ModCyclopsServer) respondWithJSON(w http.ResponseWriter, data any, caption string) error {
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("could not encode JSON for %s: %w", caption, err)
	}
	server.Log("response", string(b))

	w.Header().Set("Content-Type", "application/json")

	// If w.write fails there is no way to report this to the client: see MODREP-37.
	_, _ = w.Write(b)
	return nil
}

func readResults(resp *ccms.Response) []ccms.Result {
	results := make([]ccms.Result, 0)
	for r := range resp.Results() {
		results = append(results, r)
	}
	return results
}

// -----------------------------------------------------------------------------
// Input-sanitisation helpers for command construction.
//
// Commands are built by interpolating user-supplied values into a SQL-like
// language and sent verbatim to CCMS, which accepts multiple ';'-separated
// statements in a single request. Unvalidated interpolation therefore allows
// statement injection. Every value placed into a command must pass through one
// of these helpers according to its syntactic role.

// identRe matches a safe identifier (object/field/property name), following the
// same grammar as CCMS's Validator.Ident: a letter, then any number of letters,
// digits, '_' or '.'.
var identRe = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z_.]*$`)

// ident validates that s is a safe identifier and returns it unchanged.
func ident(caption string, s string) (string, error) {
	if !identRe.MatchString(s) {
		return "", fmt.Errorf("invalid %s identifier: %q", caption, s)
	}
	return s, nil
}

// intRe matches a decimal integer, following the same grammar as CCMS's
// Validator.Int: an optional leading '-' then one or more digits.
var intRe = regexp.MustCompile(`^-?[0-9]+$`)

// intval validates that s is a decimal integer and returns it unchanged.
func intval(s string) (string, error) {
	if !intRe.MatchString(s) {
		return "", fmt.Errorf("invalid integer: %q", s)
	}
	return s, nil
}

// sqlString renders s as a CCMS string literal: wrapped in single quotes with
// any embedded quote escaped by doubling it. Control characters have no
// representation in the grammar and are rejected.
func sqlString(s string) (string, error) {
	if strings.ContainsAny(s, "\x00\n\r") {
		return "", fmt.Errorf("illegal control character in string value")
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'", nil
}

// fieldList validates a comma-separated list of field names, also allowing the
// wildcard "*" and the aggregate "COUNT(*)".
func fieldList(s string) (string, error) {
	parts := strings.Split(s, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "*" || strings.EqualFold(p, "COUNT(*)") {
			out[i] = p
			continue
		}
		v, err := ident("field", p)
		if err != nil {
			return "", fmt.Errorf("invalid field: %q", p)
		}
		out[i] = v
	}
	return strings.Join(out, ","), nil
}

// sortList validates a comma-separated ORDER BY list: each term is a field name
// with an optional "asc"/"desc" direction.
func sortList(s string) (string, error) {
	parts := strings.Split(s, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		fields := strings.Fields(p)
		if len(fields) == 0 || len(fields) > 2 {
			return "", fmt.Errorf("invalid sort term: %q", p)
		}
		col, err := ident("sortField", fields[0])
		if err != nil {
			return "", fmt.Errorf("invalid sortField: %q", fields[0])
		}
		term := col
		if len(fields) == 2 {
			dir := strings.ToLower(fields[1])
			if dir != "asc" && dir != "desc" {
				return "", fmt.Errorf("invalid sort direction: %q", fields[1])
			}
			term += " " + dir
		}
		out[i] = term
	}
	return strings.Join(out, ","), nil
}

func mustString(v any) string {
	s, ok := v.(string)
	if !ok {
		panic("mustString: value is not a string")
	}
	return s
}
