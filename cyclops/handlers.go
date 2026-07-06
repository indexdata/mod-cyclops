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

type FilterList struct {
	Filters []any `json:"filters"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowFilters(w http.ResponseWriter, req *http.Request, caption string) error {
	resp, err := server.sendToCCMS(caption, "show filters;")
	if err != nil {
		return err
	}

	result := readResults(resp)[0]
	filters := make([]any, 0)
	for val := range result.Data() {
		filters = append(filters, val.Values()[0])
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

type SetList struct {
	Sets []any `json:"sets"`
	// No other elements yet, but use a structure for future expansion
}

func (server *ModCyclopsServer) handleShowSets(w http.ResponseWriter, req *http.Request, caption string) error {
	resp, err := server.sendToCCMS(caption, "show sets;")
	if err != nil {
		return fmt.Errorf("could not fetch show-sets response: %w", err)
	}

	result := readResults(resp)[0]
	sets := make([]any, 0)
	for val := range result.Data() {
		sets = append(sets, val.Values()[0])
	}
	setList := SetList{Sets: sets}
	return server.respondWithJSON(w, setList, caption)
}

// -----------------------------------------------------------------------------

type CreateSet struct {
	Name string `json:"name"`
}

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
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+set.Name, command)
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
		// XXX injection risk: 'cond' is a free-form condition expression and is
		// not sanitised. Safe handling needs AST-based construction (or a
		// validating parser) rather than string interpolation.
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

func makeRetrieveCommand(req *http.Request, countOnly bool) (string, error) {
	selectFields := req.URL.Query().Get("fields")
	if countOnly {
		selectFields = "COUNT(*)"
	}

	selectClause, err := makeSelectClause(
		selectFields,
		chi.URLParam(req, "setName"),
		req.URL.Query().Get("cond"),
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
	OmitTag string `json:"omittag"`
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

	clause, err := makeSelectClause(
		"*",
		params.From,
		params.Cond,
		params.Filter,
		params.Tag,
		params.OmitTag,
		"", // Sort
		params.Limit,
		"", // Offset
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
	OmitTag string `json:"omittag"`
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
	validId, err := ident("record", recordId)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	validFund, err := ident("fund", record.Fund)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := fmt.Sprintf("update %s set decision = %v where id = %s; update %s set fund = %s where id = %s;",
		validSet, record.Decision, validId, validSet, validFund, validId)
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+setName+"/"+recordId, command)
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
	AltName string `json:"altName"`
	Title   string `json:"title"`
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
			AltName: mustString(values[0]),
			Title:   mustString(values[1]),
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
	AltName      string            `json:"altName"`
	Title        string            `json:"title"`
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
		pair := strings.SplitN(segment, ":", 2)
		items[i] = ProjectItem{Id: pair[0], Name: pair[1]}
	}
	return items
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
		AltName: projectId,
	}

	for val := range result.Data() {
		pair := val.Values()
		key := mustString(pair[0])
		value := pair[1]

		switch key {
		case "title":
			project.Title = mustString(value)
		case "action":
			pair := strings.SplitN(mustString(value), ":", 2)
			project.Action = ProjectAction{
				Id:   pair[0],
				Name: pair[1],
			}
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
	if project.AltName == "" {
		return fmt.Errorf("%s: no altName specified", caption)
	}
	altName, err := ident("project", project.AltName)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	// A freshly created project has no funds yet.
	body, err := project2command(altName, project, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}
	command := "create project " + altName + ";\n" + body
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+project.AltName, command)
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

	command := "archive project " + projectId + ";"
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
	title, err := sqlString(project.Title)
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
	b.WriteString("alter project " + id + " alter property title set " + title + ";\n")
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

	result := readResults(resp)[0]
	sets := make([]any, 0)
	for val := range result.Data() {
		sets = append(sets, val.Values()[0])
	}
	setList := SetList{Sets: sets}
	return server.respondWithJSON(w, setList, caption)
}

// -----------------------------------------------------------------------------

type Fund struct {
	Name  string `json:"name"`
	Title string `json:"title"`
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
			Name:  mustString(values[0]),
			Title: mustString(values[1]),
		})
	}
	fundList := FundList{Funds: funds}
	return server.respondWithJSON(w, fundList, caption)
}

// -----------------------------------------------------------------------------

type CreateFund struct {
	Name string `json:"name"`
}

func (server *ModCyclopsServer) handleCreateFund(w http.ResponseWriter, req *http.Request, caption string) error {
	var fund CreateFund
	err := unmarshalBody(req, &fund)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	name, err := ident("fund", fund.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "create fund " + name + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+fund.Name, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) fetchFund(caption string, name string) (Fund, error) {
	command := "show fund " + name + ";"
	server.Log("command", command)
	resp, err := server.sendToCCMS(caption, command)
	if err != nil {
		return Fund{}, err
	}

	result := readResults(resp)[0]
	fund := Fund{
		Name: name,
	}

	for val := range result.Data() {
		pair := val.Values()
		key := mustString(pair[0])
		value := pair[1]

		switch key {
		case "title":
			fund.Title = mustString(value)
		default:
			server.Log("data", "unrecognised Fund field", key, "=", fmt.Sprintf("%+v", value))
		}
	}

	return fund, nil
}

func (server *ModCyclopsServer) handleFetchFund(w http.ResponseWriter, req *http.Request, caption string) error {
	name, err := ident("fund", chi.URLParam(req, "fundName"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	fund, err := server.fetchFund(caption, name)
	if err != nil {
		return err
	}

	return server.respondWithJSON(w, fund, caption)
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleUpdateFund(w http.ResponseWriter, req *http.Request, caption string) error {
	name, err := ident("fund", chi.URLParam(req, "fundName"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	var fund Fund
	err = unmarshalBody(req, &fund)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	title, err := sqlString(fund.Title)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "alter fund " + name + " alter property title set " + title + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+name, command)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func (server *ModCyclopsServer) handleDeleteFund(w http.ResponseWriter, req *http.Request, caption string) error {
	name, err := ident("fund", chi.URLParam(req, "fundName"))
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "drop fund " + name + ";"
	server.Log("command", command)

	_, err = server.sendToCCMS(caption+" "+name, command)
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

// identRe matches a safe identifier (object/field/property name): a
// dotted sequence of segments, each made of letters, digits, '_' and '-'.
var identRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)

// ident validates that s is a safe identifier and returns it unchanged.
func ident(caption string, s string) (string, error) {
	if !identRe.MatchString(s) {
		return "", fmt.Errorf("invalid %s identifier: %q", caption, s)
	}
	return s, nil
}

// intval validates that s is a decimal integer and returns its canonical form.
func intval(s string) (string, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid integer: %q", s)
	}
	return strconv.FormatInt(n, 10), nil
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
