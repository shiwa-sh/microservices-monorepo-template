// Command admin-gen scaffolds the Lowdefy admin pages (ADR-0401) from the service
// OpenAPI specs (ADR-0303). It emits apps/admin/_generated/<service>/ — a small set
// of pages per resource and per action — a shared navigation menu, and a pages.yaml
// manifest the root lowdefy.yaml references. Output is REST-connector pages only:
// every mutation goes through the service Go API, never raw SQL (the ADR-0401
// write-path invariant).
//
// The surface a service gets is driven by two markers on its spec:
//   - a tag with `x-admin: crud`  → a Django-admin-style resource: a list (changelist)
//     page, and — only when the matching operations exist — a separate "add" page
//     (POST returning 201; an async 202 workflow create is intentionally skipped) and
//     a separate "edit" page (PUT/DELETE, prefilled from GET /{id} when present).
//   - an operation with `x-admin: action` → a single button/form action page.
//
// Every page is a PageSiderMenu, so the generated menu renders as a persistent left
// nav across the whole console. Pages call the service in-cluster (connectionId ==
// service name) using the raw spec paths, so the output is independent of the /api
// edge prefix. The AxiosHttp connection returns the HTTP envelope, so response bodies
// are read at `.data` (e.g. a list grid binds `_request: list.data`).
package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	specGlob = "services/*/openapi.yaml"
	outRoot  = "apps/admin/_generated"
	adminDir = "apps/admin"

	pageType  = "PageSiderMenu"
	typeAxios = "AxiosHttp"
	// AG Grid's column keys: the row binding and the header label, both of which a
	// money column emits twice.
	keyField      = "field"
	keyHeaderName = "headerName"

	typeText   = "TextInput"
	typeNum    = "NumberInput"
	typeSwitch = "Switch"
	typePass   = "PasswordInput"
	typeButton = "Button"
	typeTitle  = "Title"
	typePara   = "Paragraph"
	typeCard   = "Card"
	typeGrid   = "AgGridAlpine"

	mediaJSON = "application/json"

	mGet    = "get"
	mPost   = "post"
	mPut    = "put"
	mDelete = "delete"

	stateKey    = "_state"
	payloadKey  = "_payload"
	requestKey  = "_request"
	concatKey   = "_string.concat"
	eventKey    = "_event"
	urlQueryKey = "_url_query"

	crudMarker   = "crud"
	actionMarker = "action"

	// listPageSize is the changelist grid's client-side page size (Django-style).
	listPageSize = 20

	menuFile = "menu.yaml"

	// Lowdefy action + menu type strings and the map keys used across page blocks.
	actLink       = "Link"
	actRequest    = "Request"
	actMessage    = "DisplayMessage"
	menuLinkType  = "MenuLink"
	menuGroupType = "MenuGroup"

	keyOnClick = "onClick"
	keyParams  = "params"
	keyPageID  = "pageId"
	keyMethod  = "method"
	keyContent = "content"
	keyTitle   = "title"
	keyURL     = "url"
	keyType    = "type"
	keyData    = "data"
)

var httpMethods = map[string]bool{
	mGet: true, mPut: true, mPost: true, mDelete: true,
	"patch": true, "options": true, "head": true, "trace": true,
}

func main() {
	err := run()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "admin-gen: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	specs, err := filepath.Glob(specGlob)
	if err != nil {
		return fmt.Errorf("glob %s: %w", specGlob, err)
	}
	sort.Strings(specs)

	err = os.RemoveAll(outRoot)
	if err != nil {
		return fmt.Errorf("clean %s: %w", outRoot, err)
	}

	var generated []string
	// The nav always opens with a hand-written dashboard link; each service then
	// contributes a menu group of its resource and action pages.
	groups := []menuLink{{
		ID: "link_dashboard", Type: menuLinkType, PageID: "dashboard",
		Properties: map[string]any{keyTitle: "Dashboard"},
	}}
	for _, specPath := range specs {
		svc := filepath.Base(filepath.Dir(specPath))
		// _template is scaffolding, not a live service — never generated for.
		if svc == "_template" {
			continue
		}
		refs, group, err := genService(svc, specPath)
		if err != nil {
			return fmt.Errorf("%s: %w", svc, err)
		}
		generated = append(generated, refs...)
		if len(group.Links) > 0 {
			groups = append(groups, group)
		}
	}

	err = writeMenu(groups)
	if err != nil {
		return err
	}
	return writeManifest(generated)
}

// genService writes every page for one service and returns their manifest refs
// (root-relative to apps/admin) plus the service's nav menu group.
func genService(svc, specPath string) ([]string, menuLink, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, menuLink{}, fmt.Errorf("read %s: %w", specPath, err)
	}
	var d doc
	err = yaml.Unmarshal(data, &d)
	if err != nil {
		return nil, menuLink{}, fmt.Errorf("parse %s: %w", specPath, err)
	}

	resources, actions, err := d.collect()
	if err != nil {
		return nil, menuLink{}, err
	}

	group := menuLink{ID: "group_" + svc, Type: menuGroupType, Properties: map[string]any{keyTitle: title(svc)}}
	var refs []string
	for _, name := range sortedKeys(resources) {
		r := resources[name]
		pageRefs, err := d.writeResourcePages(svc, r)
		if err != nil {
			return nil, menuLink{}, err
		}
		refs = append(refs, pageRefs...)
		if r.list != nil {
			link := menuLink{
				ID: "link_" + r.name, Type: menuLinkType, PageID: r.name,
				Properties: map[string]any{keyTitle: title(r.name)},
			}
			group.Links = append(group.Links, link)
		}
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].OperationID < actions[j].OperationID })
	for _, a := range actions {
		ref, err := d.writeActionPage(svc, a)
		if err != nil {
			return nil, menuLink{}, err
		}
		refs = append(refs, ref)
		link := menuLink{
			ID: "link_" + a.OperationID, Type: menuLinkType, PageID: a.OperationID,
			Properties: map[string]any{keyTitle: humanize(a.OperationID)},
		}
		group.Links = append(group.Links, link)
	}
	return refs, group, nil
}

// collect classifies a spec's operations into CRUD resource groups and standalone
// actions, both keyed and ordered deterministically by the caller.
func (d *doc) collect() (map[string]*resource, []op, error) {
	crud := d.crudTags()
	resources := map[string]*resource{}
	var actions []op

	for _, p := range sortedKeys(d.Paths) {
		methods := d.Paths[p]
		for _, m := range sortedKeys(methods) {
			if !httpMethods[m] {
				continue
			}
			var o op
			node := methods[m]
			err := node.Decode(&o)
			if err != nil {
				return nil, nil, fmt.Errorf("parse %s %s: %w", m, p, err)
			}
			o.method, o.path = m, p

			if o.XAdmin == actionMarker {
				actions = append(actions, o)
				continue
			}
			tag := o.crudTag(crud)
			if tag == "" {
				continue
			}
			r := resources[tag]
			if r == nil {
				r = &resource{name: tag}
				resources[tag] = r
			}
			r.classify(o)
		}
	}
	return resources, actions, nil
}

// writeResourcePages emits the Django-admin page set for one resource: a list
// (changelist) page always, plus separate add and edit pages when the resource
// exposes the matching operations. It returns the manifest refs it wrote.
func (d *doc) writeResourcePages(svc string, r *resource) ([]string, error) {
	if r.list == nil && r.create == nil && r.update == nil && r.remove == nil {
		return nil, nil
	}
	var refs []string
	hasEdit := r.update != nil || r.remove != nil

	if r.list != nil {
		ref, err := d.writeListPage(svc, r, hasEdit)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if r.create != nil {
		ref, err := d.writeCreatePage(svc, r)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if hasEdit {
		ref, err := d.writeEditPage(svc, r)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// writeListPage emits the changelist: a table of the resource, an "Add" button that
// links to the create page (when a create exists), and row-click navigation to the
// edit page (when an edit page exists).
func (d *doc) writeListPage(svc string, r *resource, hasEdit bool) (string, error) {
	pg := page{ID: r.name, Type: pageType, Properties: pageProps{Title: title(r.name)}}

	req := request{ID: "list", Type: typeAxios, ConnectionID: svc}
	req.Properties = map[string]any{keyURL: r.list.path, keyMethod: mGet}
	pg.Requests = append(pg.Requests, req)

	// Lowdefy 5 does not auto-run a request just because a block binds it; fetch the
	// list explicitly on mount so the grid is populated when the page paints.
	pg.Events = map[string]any{"onMount": []any{
		map[string]any{"id": "loadList", keyType: actRequest, keyParams: "list"},
	}}

	pg.Blocks = append(pg.Blocks, heading(title(r.name)))
	if r.create != nil {
		pg.Blocks = append(pg.Blocks, linkButton("add", "Add "+singular(r.name), r.name+"_new", nil, "primary"))
	}

	table := block{ID: "table", Type: typeGrid}
	table.Properties = map[string]any{
		"rowData":    map[string]any{requestKey: "list.data"},
		"columnDefs": d.columnDefs(r.list.responseSchema()),
		// Every changelist paginates, like Django admin. AgGrid paginates the loaded
		// page client-side; the list request itself is a single server page (services
		// that support it accept page/per_page — see authz listIdentities).
		"pagination":         true,
		"paginationPageSize": listPageSize,
	}
	if hasEdit {
		// Django's changelist: click a row to open its change page. The AgGrid
		// onRowClick event carries the row under `event.row`.
		table.Events = map[string]any{"onRowClick": []any{
			map[string]any{
				"id": "openRow", keyType: actLink,
				keyParams: map[string]any{
					keyPageID:  r.name + "_edit",
					"urlQuery": map[string]any{"id": map[string]any{eventKey: "row.id"}},
				},
			},
		}}
		pg.Blocks = append(pg.Blocks, hint("Click a row to edit."))
	}
	pg.Blocks = append(pg.Blocks, card(r.name+"_card", title(r.name), []block{table}))
	return writePage(svc, r.name, pg)
}

// writeCreatePage emits the standalone "add" form for a resource.
func (d *doc) writeCreatePage(svc string, r *resource) (string, error) {
	pg := page{ID: r.name + "_new", Type: pageType, Properties: pageProps{Title: "Add " + singular(r.name)}}
	pg.Blocks = append(pg.Blocks, heading("Add "+singular(r.name)))

	fields := d.props(r.create.bodySchema())
	payload := map[string]any{}
	data := map[string]any{}
	form := make([]block, 0, len(fields)+3)
	for _, f := range fields {
		form = append(form, inputsFor(f)...)
		payload[f.Name] = map[string]any{stateKey: f.Name}
		data[f.Name] = map[string]any{payloadKey: f.Name}
	}
	req := request{
		ID: "create", Type: typeAxios, ConnectionID: svc,
		Payload:    payload,
		Properties: map[string]any{keyURL: pathToURL(r.create.path), keyMethod: mPost, keyData: data},
	}
	pg.Requests = append(pg.Requests, req)

	// Django's add flow: on success return to the changelist, where the new row is
	// the confirmation (a toast announces it). A failed request throws and halts the
	// chain, so neither the toast nor the redirect fires.
	toast := message("Created " + singular(r.name))
	submit := requestButton("submit", "Create", "create", "primary", false, toast, linkTo(r.name))
	back := linkButton("back", "Back to list", r.name, nil, "")
	form = append(form, submit, back)
	pg.Blocks = append(pg.Blocks, card(r.name+"_form", "New "+singular(r.name), form))
	return writePage(svc, r.name+"_new", pg)
}

// writeEditPage emits the standalone change page: a form prefilled from GET /{id}
// (when present), a Save (PUT), and a Delete (DELETE) that returns to the list.
func (d *doc) writeEditPage(svc string, r *resource) (string, error) {
	pg := page{ID: r.name + "_edit", Type: pageType, Properties: pageProps{Title: "Edit " + singular(r.name)}}
	pg.Blocks = append(pg.Blocks, heading("Edit "+singular(r.name)))

	// The record id comes from the ?id= query the changelist row-click set; the
	// editable fields are whatever the PUT body accepts (none for a delete-only
	// resource, which then renders just the id and a Delete button).
	idFromQuery := map[string]any{urlQueryKey: "id"}
	var fields []prop
	if r.update != nil {
		fields = d.props(r.update.bodySchema())
	}

	d.appendPrefill(&pg, svc, r, fields, idFromQuery)

	form := make([]block, 0, len(fields)+5)
	form = append(form, readonlyID(idFromQuery))
	for _, f := range fields {
		form = append(form, inputsFor(f)...)
	}
	form = append(form, d.editWrites(&pg, svc, r, fields, idFromQuery)...)
	form = append(form, linkButton("back", "Back to list", r.name, nil, ""))

	pg.Blocks = append(pg.Blocks, card(r.name+"_form", "Edit "+singular(r.name), form))
	return writePage(svc, r.name+"_edit", pg)
}

// appendPrefill wires the change page to load its record on mount (GET /{id}) and
// copy the response body into the form's field state. No-op when the resource has
// no get-by-id — the form then starts blank.
func (d *doc) appendPrefill(pg *page, svc string, r *resource, fields []prop, id map[string]any) {
	if r.get == nil {
		return
	}
	req := request{
		ID: "get", Type: typeAxios, ConnectionID: svc,
		Payload:    map[string]any{"id": id},
		Properties: map[string]any{keyURL: pathToURL(r.get.path), keyMethod: mGet},
	}
	pg.Requests = append(pg.Requests, req)
	fill := map[string]any{}
	for _, f := range fields {
		fill[f.Name] = map[string]any{requestKey: "get.data." + f.Name}
	}
	pg.Events = map[string]any{"onMount": []any{
		map[string]any{"id": "load", keyType: actRequest, keyParams: "get"},
		map[string]any{"id": "fill", keyType: "SetState", keyParams: fill},
	}}
}

// editWrites appends the change page's Save (PUT) and Delete (DELETE) requests and
// returns the buttons that drive them.
func (d *doc) editWrites(pg *page, svc string, r *resource, fields []prop, id map[string]any) []block {
	var controls []block
	if r.update != nil {
		payload := map[string]any{"id": id}
		data := map[string]any{}
		for _, f := range fields {
			payload[f.Name] = map[string]any{stateKey: f.Name}
			data[f.Name] = map[string]any{payloadKey: f.Name}
		}
		req := request{
			ID: "update", Type: typeAxios, ConnectionID: svc,
			Payload:    payload,
			Properties: map[string]any{keyURL: pathToURL(r.update.path), keyMethod: mPut, keyData: data},
		}
		pg.Requests = append(pg.Requests, req)
		// Save keeps the operator on the record (Django "save and continue"), with a
		// toast confirming the write.
		save := requestButton("save", "Save", "update", "primary", false, message("Saved changes"))
		controls = append(controls, save)
	}
	if r.remove != nil {
		req := request{
			ID: "remove", Type: typeAxios, ConnectionID: svc,
			Payload:    map[string]any{"id": id},
			Properties: map[string]any{keyURL: pathToURL(r.remove.path), keyMethod: mDelete},
		}
		pg.Requests = append(pg.Requests, req)
		// Delete, then return to the list — the record no longer exists to edit.
		del := requestButton("delete", "Delete", "remove", "", true, linkTo(r.name))
		controls = append(controls, del)
	}
	return controls
}

// writeActionPage emits a single action page: inputs for the operation's path params
// and request body, and a button that posts to the endpoint.
func (d *doc) writeActionPage(svc string, o op) (string, error) {
	pg := page{ID: o.OperationID, Type: pageType, Properties: pageProps{Title: humanize(o.OperationID)}}
	pg.Blocks = append(pg.Blocks, heading(humanize(o.OperationID)))

	payload := map[string]any{}
	data := map[string]any{}
	form := make([]block, 0, len(o.pathParams())+2)
	for _, name := range o.pathParams() {
		form = append(form, input(name, name, "string"))
		payload[name] = map[string]any{stateKey: name}
	}
	for _, p := range d.props(o.bodySchema()) {
		form = append(form, input(p.Name, p.Name, p.Type))
		payload[p.Name] = map[string]any{stateKey: p.Name}
		data[p.Name] = map[string]any{payloadKey: p.Name}
	}
	props := map[string]any{keyURL: pathToURL(o.path), keyMethod: o.method}
	if len(data) > 0 {
		props["data"] = data
	}
	req := request{ID: "run", Type: typeAxios, ConnectionID: svc, Properties: props}
	if len(payload) > 0 {
		req.Payload = payload
	}
	pg.Requests = append(pg.Requests, req)

	label := humanize(o.OperationID)
	submit := requestButton("submit", label, "run", "primary", false, message(label+" succeeded"))
	form = append(form, submit)
	pg.Blocks = append(pg.Blocks, card(o.OperationID+"_form", label, form))
	return writePage(svc, o.OperationID, pg)
}

// ── block builders ──────────────────────────────────────────────────────────────

func heading(text string) block {
	return block{ID: "header", Type: typeTitle, Properties: map[string]any{keyContent: text, "level": 3}}
}

func hint(text string) block {
	return block{ID: "hint", Type: typePara, Properties: map[string]any{keyContent: text}}
}

// card wraps blocks in a titled antd Card — the panel that gives each page its
// clean, single-purpose framing.
func card(id, title string, blocks []block) block {
	return block{ID: id, Type: typeCard, Properties: map[string]any{keyTitle: title}, Blocks: blocks}
}

// readonlyID shows the record id being edited, read from the page's ?id= query.
func readonlyID(id any) block {
	return block{ID: "id_display", Type: typePara, Properties: map[string]any{
		keyContent: map[string]any{concatKey: []any{"Editing id: ", id}},
	}}
}

// linkButton is a Button whose onClick navigates to another page.
func linkButton(id, label, pageID string, urlQuery map[string]any, btnType string) block {
	params := map[string]any{keyPageID: pageID}
	if urlQuery != nil {
		params["urlQuery"] = urlQuery
	}
	b := block{ID: id, Type: typeButton, Style: buttonSpacing(), Properties: map[string]any{keyTitle: label}}
	if btnType != "" {
		b.Properties["type"] = btnType
	}
	b.Events = map[string]any{keyOnClick: []any{
		map[string]any{"id": id + actLink, keyType: actLink, keyParams: params},
	}}
	return b
}

// buttonSpacing separates a button from the field or button above it — Lowdefy
// stacks blocks flush, so form actions need explicit top margin to breathe.
func buttonSpacing() map[string]any {
	return map[string]any{"marginTop": 16, "marginRight": 8}
}

// requestButton runs reqID on click, then the follow-up steps — a success toast
// and/or a navigation. A failed request throws and halts the chain (ADR-0601), so
// the follow-ups only run on success. Feedback is a transient DisplayMessage toast
// rather than a state-gated block: Lowdefy only hides a block on a strictly-false
// visible, which an unset flag never yields, so a toast is both simpler and correct.
func requestButton(id, label, reqID, btnType string, danger bool, follow ...map[string]any) block {
	b := block{ID: id, Type: typeButton, Style: buttonSpacing(), Properties: map[string]any{keyTitle: label}}
	if btnType != "" {
		b.Properties[keyType] = btnType
	}
	if danger {
		b.Properties["danger"] = true
	}
	onClick := make([]any, 0, 1+len(follow))
	onClick = append(onClick, map[string]any{"id": id + "Req", keyType: actRequest, keyParams: reqID})
	for i, step := range follow {
		step["id"] = fmt.Sprintf("%s%d", id, i)
		onClick = append(onClick, step)
	}
	b.Events = map[string]any{keyOnClick: onClick}
	return b
}

// message is an onClick step showing a success toast.
func message(text string) map[string]any {
	return map[string]any{keyType: actMessage, keyParams: map[string]any{keyContent: text, "status": "success"}}
}

// linkTo is an onClick step navigating to a page.
func linkTo(pageID string) map[string]any {
	return map[string]any{keyType: actLink, keyParams: map[string]any{keyPageID: pageID}}
}

func (d *doc) columnDefs(schema yaml.Node) []any {
	cols := d.props(schema)
	defs := make([]any, 0, len(cols))
	for _, c := range cols {
		// A money column is two columns. AG Grid reads a dotted field as a path into
		// the row, so this needs no expression and no formatter — and showing the
		// currency as its own column is what a changelist of mixed currencies needs.
		if c.isMoney() {
			defs = append(
				defs,
				map[string]any{keyField: c.Name + ".amount", keyHeaderName: headerName(c.Name)},
				map[string]any{keyField: c.Name + ".currency", keyHeaderName: "Currency"},
			)
			continue
		}
		defs = append(defs, map[string]any{keyField: c.Name, keyHeaderName: headerName(c.Name)})
	}
	return defs
}

// inputsFor is the form side of the same case. A dotted block id is nested state in
// Lowdefy, so `price.amount` and `price.currency` assemble the object the request
// payload already reads as `price` — the two fields a human fills, and the one
// object the API takes.
//
// The amount is a TextInput and not a NumberInput deliberately: a NumberInput hands
// back a JavaScript number, which is the double the string form exists to avoid.
func inputsFor(f prop) []block {
	if !f.isMoney() {
		return []block{input(f.Name, f.Name, f.Type)}
	}
	return []block{
		input(f.Name+".amount", f.Name, "string"),
		input(f.Name+".currency", "currency", "string"),
	}
}

func input(state, label, typ string) block {
	// Lowdefy's `label` is an OBJECT, not a string: a bare string is ignored and the
	// field falls back to labelling itself with its own block id. That was invisible
	// while every id was already the field name ("price" labelled `price`), and
	// stopped being invisible the moment a money field's id became `price.amount`.
	b := block{ID: state, Properties: map[string]any{"label": map[string]any{"title": headerName(label)}}}
	switch typ {
	case "integer", "number":
		b.Type = typeNum
	case "boolean":
		b.Type = typeSwitch
	default:
		if label == "password" {
			b.Type = typePass
		} else {
			b.Type = typeText
		}
	}
	return b
}

// ── spec model ────────────────────────────────────────────────────────────────

type doc struct {
	Tags       []tagDef                        `yaml:"tags"`
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Components struct {
		Schemas map[string]yaml.Node `yaml:"schemas"`
	} `yaml:"components"`
}

type tagDef struct {
	Name   string `yaml:"name"`
	XAdmin string `yaml:"x-admin"`
}

type param struct {
	Name string `yaml:"name"`
	In   string `yaml:"in"`
}

type mediaSchema struct {
	Schema yaml.Node `yaml:"schema"`
}

type body struct {
	Content map[string]mediaSchema `yaml:"content"`
}

type op struct {
	OperationID string          `yaml:"operationId"`
	XAdmin      string          `yaml:"x-admin"`
	Tags        []string        `yaml:"tags"`
	Parameters  []param         `yaml:"parameters"`
	RequestBody body            `yaml:"requestBody"`
	Responses   map[string]body `yaml:"responses"`

	method string
	path   string
}

func (d *doc) crudTags() map[string]bool {
	out := map[string]bool{}
	for _, t := range d.Tags {
		if t.XAdmin == crudMarker {
			out[t.Name] = true
		}
	}
	return out
}

// crudTag returns the operation's first tag that is a CRUD resource group.
func (o *op) crudTag(crud map[string]bool) string {
	for _, t := range o.Tags {
		if crud[t] {
			return t
		}
	}
	return ""
}

func (o *op) pathParams() []string {
	var out []string
	for _, p := range o.Parameters {
		if p.In == "path" {
			out = append(out, p.Name)
		}
	}
	return out
}

func (o *op) bodySchema() yaml.Node {
	return o.RequestBody.Content[mediaJSON].Schema
}

func (o *op) responseSchema() yaml.Node {
	b, ok := o.Responses["200"]
	if !ok {
		b = o.Responses["201"]
	}
	return b.Content[mediaJSON].Schema
}

// segments counts the non-empty path segments (e.g. /orders/{id} == 2).
func (o *op) segments() int {
	return len(strings.FieldsFunc(o.path, func(r rune) bool { return r == '/' }))
}

func (o *op) has201() bool {
	_, ok := o.Responses["201"]
	return ok
}

// resource collects the CRUD operations of one resource group.
type resource struct {
	name                              string
	list, get, create, update, remove *op
}

// classify assigns an operation to its CRUD role by method and path shape. An
// async create (POST returning 202, not 201) is intentionally left unassigned:
// it is a workflow trigger, not a form-scaffoldable create.
func (r *resource) classify(o op) {
	cp := o
	switch {
	case len(o.pathParams()) == 0 && o.method == mGet:
		r.list = &cp
	case o.segments() == 2 && o.method == mGet:
		r.get = &cp
	case len(o.pathParams()) == 0 && o.method == mPost && o.has201():
		r.create = &cp
	case o.segments() == 2 && o.method == mPut:
		r.update = &cp
	case o.segments() == 2 && o.method == mDelete:
		r.remove = &cp
	}
}

type prop struct {
	Name, Type, Format string
	// Ref is the component this property points at, when it is a $ref rather than an
	// inline type. It is what makes a shared shape recognisable here: `Money` is an
	// object, and an object rendered by the default path is a TextInput that posts
	// "[object Object]".
	Ref string
}

// isMoney reports whether the property is the shared Money component (ADR-0300):
// an amount as a decimal STRING plus its currency. It is the one shared shape this
// generator special-cases, because it is the one that is structurally an object and
// semantically a single field a human types into.
func (p prop) isMoney() bool { return path.Base(p.Ref) == "Money" }

// props resolves a schema node to its ordered properties, following a $ref into
// components and unwrapping an array to its item schema.
func (d *doc) props(schema yaml.Node) []prop {
	if schema.Kind == 0 {
		return nil
	}
	var s struct {
		Ref        string    `yaml:"$ref"`
		Type       string    `yaml:"type"`
		Items      yaml.Node `yaml:"items"`
		Properties yaml.Node `yaml:"properties"`
	}
	err := schema.Decode(&s)
	if err != nil {
		return nil
	}
	switch {
	case s.Ref != "":
		node, ok := d.Components.Schemas[path.Base(s.Ref)]
		if !ok {
			return nil
		}
		return d.props(node)
	case s.Type == "array":
		return d.props(s.Items)
	}
	return orderedProps(s.Properties)
}

// orderedProps reads a `properties` mapping node in source order.
func orderedProps(node yaml.Node) []prop {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]prop, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		var v struct {
			Type   string `yaml:"type"`
			Format string `yaml:"format"`
			Ref    string `yaml:"$ref"`
		}
		_ = node.Content[i+1].Decode(&v)
		out = append(out, prop{Name: node.Content[i].Value, Type: v.Type, Format: v.Format, Ref: v.Ref})
	}
	return out
}

// ── Lowdefy page model ────────────────────────────────────────────────────────

type page struct {
	ID         string         `yaml:"id"`
	Type       string         `yaml:"type"`
	Properties pageProps      `yaml:"properties"`
	Events     map[string]any `yaml:"events,omitempty"`
	Requests   []request      `yaml:"requests,omitempty"`
	Blocks     []block        `yaml:"blocks"`
}

type pageProps struct {
	Title string `yaml:"title"`
}

type request struct {
	ID           string         `yaml:"id"`
	Type         string         `yaml:"type"`
	ConnectionID string         `yaml:"connectionId"`
	Payload      map[string]any `yaml:"payload,omitempty"`
	Properties   map[string]any `yaml:"properties"`
}

type block struct {
	ID         string         `yaml:"id"`
	Type       string         `yaml:"type"`
	Properties map[string]any `yaml:"properties,omitempty"`
	Style      map[string]any `yaml:"style,omitempty"`
	Events     map[string]any `yaml:"events,omitempty"`
	Blocks     []block        `yaml:"blocks,omitempty"`
}

// menuLink is a Lowdefy nav entry: a MenuLink (leaf, links to a pageId) or a
// MenuGroup (a titled group of links).
type menuLink struct {
	ID         string         `yaml:"id"`
	Type       string         `yaml:"type"`
	PageID     string         `yaml:"pageId,omitempty"`
	Properties map[string]any `yaml:"properties,omitempty"`
	Links      []menuLink     `yaml:"links,omitempty"`
}

const genHeader = "# GENERATED by tools/admin-gen (ADR-0401). Do not edit; run `mise run gen:admin`.\n"

func writePage(svc, name string, pg page) (string, error) {
	rel := filepath.Join("_generated", svc, name+".yaml")
	err := marshalFile(filepath.Join(adminDir, rel), pg)
	if err != nil {
		return "", err
	}
	return rel, nil
}

// writeMenu emits the shared nav as a single default menu the PageSiderMenu pages
// render. The root lowdefy.yaml references it with `menus: { _ref: _generated/menu.yaml }`.
func writeMenu(groups []menuLink) error {
	menus := []any{map[string]any{"id": "default", "links": groups}}
	return marshalFile(filepath.Join(outRoot, menuFile), menus)
}

func writeManifest(generated []string) error {
	static, err := staticRefs()
	if err != nil {
		return err
	}
	all := make([]string, 0, len(static)+len(generated))
	all = append(all, static...)
	all = append(all, generated...)
	var refs []any
	for _, r := range all {
		refs = append(refs, map[string]any{"_ref": r})
	}
	return marshalFile(filepath.Join(outRoot, "pages.yaml"), refs)
}

// staticRefs lists the hand-written pages (pages/ and custom/) so the manifest is
// the single page index the root config references.
func staticRefs() ([]string, error) {
	var out []string
	for _, dir := range []string{"pages", "custom"} {
		top, err := filepath.Glob(filepath.Join(adminDir, dir, "*.yaml"))
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", dir, err)
		}
		nested, err := filepath.Glob(filepath.Join(adminDir, dir, "*", "*.yaml"))
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", dir, err)
		}
		matches := make([]string, 0, len(top)+len(nested))
		matches = append(matches, top...)
		matches = append(matches, nested...)
		sort.Strings(matches)
		for _, m := range matches {
			rel, err := filepath.Rel(adminDir, m)
			if err != nil {
				return nil, fmt.Errorf("rel %s: %w", m, err)
			}
			out = append(out, rel)
		}
	}
	return out, nil
}

func marshalFile(p string, v any) error {
	err := os.MkdirAll(filepath.Dir(p), 0o750)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	var buf bytes.Buffer
	_, _ = buf.WriteString(genHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	err = enc.Encode(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", p, err)
	}
	err = enc.Close()
	if err != nil {
		return fmt.Errorf("close %s: %w", p, err)
	}
	err = os.WriteFile(p, buf.Bytes(), 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// pathToURL turns a spec path into a Lowdefy url: a plain string, or a
// _string.concat that splices path params from the request payload.
func pathToURL(p string) any {
	if !strings.Contains(p, "{") {
		return p
	}
	var parts []any
	cur := ""
	for i := 0; i < len(p); {
		if p[i] == '{' {
			j := strings.IndexByte(p[i:], '}')
			if cur != "" {
				parts = append(parts, cur)
				cur = ""
			}
			parts = append(parts, map[string]any{payloadKey: p[i+1 : i+j]})
			i += j + 1
			continue
		}
		cur += string(p[i])
		i++
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return map[string]any{concatKey: parts}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// title capitalizes a lower-case resource noun ("products" -> "Products").
func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// singular turns a plural resource noun into its per-record label: "identities" ->
// "identity" ("ies" -> "y"), "products" -> "product" (drop trailing "s"). Good
// enough for the demo resource nouns; not a full inflector.
func singular(s string) string {
	if strings.HasSuffix(s, "ies") && len(s) > 3 {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "s") && len(s) > 1 {
		return s[:len(s)-1]
	}
	return s
}

// headerName turns a snake_case field into a label ("price_cents" -> "Price cents").
func headerName(s string) string {
	return title(strings.ReplaceAll(s, "_", " "))
}

// humanize turns a camelCase operationId into words ("refundCharge" -> "Refund charge").
func humanize(s string) string {
	var out []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, ' ', r+('a'-'A'))
			continue
		}
		out = append(out, r)
	}
	return title(string(out))
}
