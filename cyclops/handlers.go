package cyclops

import "errors"
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

	command := "define tag " + tag.Name
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption+" "+tag.Name, command)
	if err != nil {
		return err
	}
	fmt.Printf("%s response: %+v\n", caption, resp)

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

type DefineFilter struct {
	Name     string `json:"name"`
	Cond     string `json:"cond"`
	Template string `json:"template"`
}

func (server *ModCyclopsServer) handleDefineFilter(w http.ResponseWriter, req *http.Request, caption string) error {
	var filter DefineFilter
	err := unmarshalBody(req, &filter)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := "define filter " + filter.Name
	if filter.Cond != "" {
		command += " where " + filter.Cond
	}
	if filter.Template != "" {
		command += " template " + filter.Template
	}
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption+" "+filter.Name, command)
	if err != nil {
		return err
	}
	fmt.Printf("%s response: %+v\n", caption, resp)

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

	command := "create set " + set.Name + ";"
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption+" "+set.Name, command)
	if err != nil {
		return err
	}
	fmt.Printf("%s response: %+v\n", caption, resp)

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// -----------------------------------------------------------------------------

func makeConditionalClause(cond, filter, tag, omitTag, sort, limit, offset string) (string, error) {
	var b strings.Builder

	if cond != "" {
		b.WriteString(" where ")
		b.WriteString(cond)
	}

	if filter != "" {
		b.WriteString(" filter ")
		b.WriteString(filter)
	}

	if tag != "" && omitTag != "" {
		return "", errors.New("both 'tag' and 'omitTag' parameters supplied")
	}

	if tag != "" {
		b.WriteString(" tag ")
		b.WriteString(tag)
	} else if omitTag != "" {
		b.WriteString(" tag not ")
		b.WriteString(omitTag)
	}

	if sort != "" {
		b.WriteString(" order by ")
		b.WriteString(sort)
	}

	if limit != "*" {
		if limit == "" {
			limit = "100"
		}
		b.WriteString(" limit ")
		b.WriteString(limit)
	}

	if offset != "" {
		b.WriteString(" offset ")
		b.WriteString(offset)
	}

	return b.String(), nil
}

func makeSelectClause(fields, setName, cond, filter, tag, omitTag, sort, limit, offset string) (string, error) {
	if fields == "" {
		return "", errors.New("no 'fields' parameter supplied")
	}

	conditionalClause, err := makeConditionalClause(cond, filter, tag, omitTag, sort, limit, offset)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	b.WriteString("select ")
	b.WriteString(fields)

	b.WriteString(" from ")
	b.WriteString(setName)

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
	command := "drop set " + chi.URLParam(req, "setName") + ";"
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption+" "+chi.URLParam(req, "setName"), command)
	if err != nil {
		return err
	}

	localrr := ccms2local(resp)
	return server.respondWithJSON(w, localrr, caption)
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
	setName := chi.URLParam(req, "setName")

	var params AddRecords
	err := unmarshalBody(req, &params)
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

	resp, err := server.sendToCCMS(caption+" "+setName, command)
	if err != nil {
		return err
	}
	fmt.Printf("%s response: %+v\n", caption, resp)

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
	setName := chi.URLParam(req, "setName")

	var params RemoveRecords
	err := unmarshalBody(req, &params)
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

	resp, err := server.sendToCCMS(caption+" "+setName, command)
	if err != nil {
		return err
	}
	fmt.Printf("%s response: %+v\n", caption, resp)

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
		altName := mustString(val.Values()[0])
		bf := BriefProject{
			AltName: altName,
		}
		projects = append(projects, bf)
	}

	projectList := ProjectList{Projects: projects}
	return server.respondWithJSON(w, projectList, caption)
}

// -----------------------------------------------------------------------------

type ProjectAction struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ProjectFund struct {
	Id   string
	Name string
}

type ProjectPerson struct {
	XId  string
	Role string
}

type ProjectLocation struct {
	Id   string
	Name string
}

type ProjectTrack struct {
	Id   string
	Name string
}

type Project struct {
	Id      string        `json:"id"`
	AltName string        `json:"altName"`
	Title   string        `json:"title"`
	Action  ProjectAction `json:"action"`
	MouLink string        `json:"mou_link"`
	Funds   string        `json:"funds"`
	// The line about should be: Funds []ProjectFund
	People    []ProjectPerson
	Locations []ProjectLocation
	Tracks    []ProjectTrack
}

func (server *ModCyclopsServer) handleFetchProject(w http.ResponseWriter, req *http.Request, caption string) error {
	projectId := chi.URLParam(req, "projectId")
	resp, err := server.sendToCCMS(caption, "show project "+projectId+";")
	if err != nil {
		return err
	}

	result := readResults(resp)[0]
	project := Project{
		AltName: projectId,
	}

	i := 0
	for val := range result.Data() {
		i += 1
		pair := val.Values()
		key := mustString(pair[0])
		value := pair[1]

		switch key {
		case "title":
			project.Title = mustString(value)
		case "action":
			project.Action = ProjectAction{
				Name: mustString(value),
			}
		case "mou_link":
			project.MouLink = mustString(value)
		case "funds":
			// XXX This should not be a string, but an array of structures
			// If necessary I can parse the string, but it would be nice not to
			project.Funds = mustString(value)
		default:
			server.Log("data", "unrecognised Project field", key)
		}
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

	command := "create project " + project.AltName + ";\n" + project2command(project.AltName, project)
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption+" "+project.AltName, command)
	if err != nil {
		return err
	}
	fmt.Printf("%s response: %+v\n", caption, resp)

	// XXX now set the fields

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (server *ModCyclopsServer) handleDeleteProject(w http.ResponseWriter, req *http.Request, caption string) error {
	w.WriteHeader(http.StatusNotImplemented)
	return nil
}

// -----------------------------------------------------------------------------

func project2command(projectId string, project Project) string {
	var b strings.Builder
	b.WriteString("alter project " + projectId + " alter property title set '" + project.Title + "';\n")
	b.WriteString("alter project " + projectId + " alter property action set '" + project.Action.Name + "';\n")
	b.WriteString("alter project " + projectId + " alter property mou_link set '" + project.MouLink + "';\n")
	b.WriteString("alter project " + projectId + " alter property funds set '" + project.Funds + "';")
	// No point supporting the next two until we know what CCMS is going to do with them
	// b.WriteString("alter project " + projectId + " alter property locations set '" + project.Locations + "';\n")
	// b.WriteString("alter project " + projectId + " alter property tracks set '" + project.Tracks + "'\n")
	return b.String()
}

func (server *ModCyclopsServer) handleUpdateProject(w http.ResponseWriter, req *http.Request, caption string) error {
	projectId := chi.URLParam(req, "projectId")
	var project Project
	err := unmarshalBody(req, &project)
	if err != nil {
		return fmt.Errorf("%s: %w", caption, err)
	}

	command := project2command(projectId, project)
	server.Log("command", command)

	resp, err := server.sendToCCMS(caption+" "+project.AltName, command)
	if err != nil {
		return err
	}
	fmt.Printf("%s response: %+v\n", caption, resp)

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

	result := readResults(resp)[0]
	if result.Status() == "error" {
		return nil, fmt.Errorf("%s failed: %s", caption, result.Message())
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

func mustString(v any) string {
	s, ok := v.(string)
	if !ok {
		panic("mustString: value is not a string")
	}
	return s
}
