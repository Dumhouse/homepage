package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dawidof/home-services/internal/changesets"
	"github.com/dawidof/home-services/internal/checks"
	"github.com/dawidof/home-services/internal/discovery"
	"github.com/dawidof/home-services/internal/icons"
	"github.com/dawidof/home-services/internal/integrations"
	"github.com/dawidof/home-services/internal/inventory"
)

// api exposes the inventory as JSON under /api/v1. The UI (htmx) and tools can both use it.
// Token scopes (read/write) arrive with the MCP milestone; until then it sits behind UI auth only.
type api struct {
	inv   *inventory.Store
	chk   *checks.Scheduler
	icons *icons.Fetcher
	intg  *integrations.Manager
	facts FactsCollector
	cs    *changesets.Store
	scan  *discovery.Runner
}

func (a *api) routes(r chi.Router) {
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/hosts", a.listHosts)
		r.Post("/hosts", a.createHost)
		r.Get("/hosts/{id}", a.getHost)
		r.Put("/hosts/{id}", a.updateHost)
		r.Patch("/hosts/{id}", a.patchHost)
		r.Delete("/hosts/{id}", a.deleteHost)
		r.Post("/hosts/{id}/restore", a.restoreHost)
		r.Post("/hosts/{id}/services", a.addService)
		r.Get("/hosts/{id}/free-port", a.freePort)
		r.Post("/hosts/{id}/facts", a.collectFacts)

		r.Put("/services/{id}", a.updateService)
		r.Delete("/services/{id}", a.deleteService)
		r.Post("/services/{id}/restore", a.restoreService)

		r.Get("/projects", a.listProjects)
		r.Post("/projects", a.createProject)
		r.Get("/projects/{id}", a.getProject)
		r.Put("/projects/{id}", a.updateProject)
		r.Delete("/projects/{id}", a.deleteProject)
		r.Post("/projects/{id}/restore", a.restoreProject)
		r.Post("/projects/{id}/links", a.addProjectLink)
		r.Post("/projects/{id}/envs", a.addProjectEnv)
		r.Post("/projects/{id}/app-links", a.addAppLink)
		r.Post("/projects/{id}/hosts", a.linkProjectHost)
		r.Post("/projects/{id}/services", a.linkProjectService)
		r.Post("/projects/{id}/connections", a.createConnection)

		r.Delete("/links/{id}", a.deleteProjectLink)
		r.Delete("/envs/{id}", a.deleteProjectEnv)
		r.Delete("/app-links/{id}", a.deleteAppLink)
		r.Delete("/connections/{id}", a.deleteConnection)

		r.Get("/locations", a.listLocations)
		r.Post("/locations", a.createLocation)
		r.Get("/locations/{id}", a.getLocation)
		r.Put("/locations/{id}", a.updateLocation)
		r.Delete("/locations/{id}", a.deleteLocation)
		r.Post("/locations/{id}/restore", a.restoreLocation)

		r.Get("/tags", a.listTags)
		r.Get("/groups", a.listGroups)

		r.Get("/access", a.listAccess)
		r.Post("/access", a.createAccess)
		r.Delete("/access/{id}", a.deleteAccess)

		r.Get("/used-by", a.usedBy)

		r.Get("/costs", a.listCosts)
		r.Post("/costs", a.createCost)
		r.Get("/costs/{id}", a.getCost)
		r.Put("/costs/{id}", a.updateCost)
		r.Delete("/costs/{id}", a.deleteCost)
		r.Post("/costs/{id}/restore", a.restoreCost)

		r.Get("/domains", a.listDomains)
		r.Post("/domains", a.createDomain)
		r.Get("/domains/{id}", a.getDomain)
		r.Put("/domains/{id}", a.updateDomain)
		r.Delete("/domains/{id}", a.deleteDomain)
		r.Post("/domains/{id}/restore", a.restoreDomain)
		r.Post("/domains/{id}/records", a.addDomainRecord)
		r.Delete("/records/{id}", a.deleteDomainRecord)

		r.Get("/checks", a.listChecks)
		r.Post("/checks", a.createCheck)
		r.Post("/checks/{id}/run", a.runCheck)
		r.Delete("/checks/{id}", a.deleteCheck)
		r.Get("/problems", a.problems)

		r.Get("/discovery", a.listDiscovery)
		r.Get("/discovery/status", a.discoveryStatus)
		r.Post("/discovery/scan", a.scanDiscovery)
		r.Post("/discovery/{id}/ignore", a.ignoreDiscovered)
		r.Post("/discovery/{id}/register", a.registerDiscovered)

		r.Get("/changesets", a.listChangesets)
		r.Post("/changesets", a.createChangeset)
		r.Get("/changesets/{id}", a.getChangeset)
		r.Post("/changesets/{id}/apply", a.applyChangeset)
		r.Post("/changesets/{id}/reject", a.rejectChangeset)
		r.Post("/changesets/{id}/undo", a.undoChangeset)

		r.Get("/integrations", a.listIntegrations)
		r.Put("/integrations/{id}", a.saveIntegration)
		r.Post("/integrations/{id}/test", a.testIntegration)
		r.Post("/integrations/{id}/sync", a.syncIntegration)

		r.Get("/search", a.search)
		r.Get("/audit", a.listAudit)
		r.Get("/export", a.export)
		r.Post("/import", a.importData)
	})
}

// projectID reads the {id} of a project route.
func projectID(r *http.Request) int64 { return id(r) }

// queryInt reads an integer query parameter.
func queryInt(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return n
}

func id(r *http.Request) int64 { n, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); return n }

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inventory.ErrNotFound):
		reply(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, inventory.ErrInvalid):
		reply(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	default:
		reply(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func decode[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		reply(w, http.StatusBadRequest, map[string]string{"error": "bad JSON: " + err.Error()})
		return v, false
	}
	return v, true
}

// result writes the JSON reply for a store call: 200/201 with the value, or the mapped error.
func result[T any](w http.ResponseWriter, status int, v T, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, status, v)
}

func (a *api) listHosts(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListHosts(r.Context())
	result(w, 200, v, err)
}
func (a *api) getHost(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.GetHost(r.Context(), id(r))
	result(w, 200, v, err)
}
func (a *api) createHost(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Host](w, r); ok {
		v, err := a.inv.CreateHost(r.Context(), actorOf(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) updateHost(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Host](w, r); ok {
		v, err := a.inv.UpdateHost(r.Context(), actorOf(r), id(r), in)
		result(w, 200, v, err)
	}
}
func (a *api) patchHost(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.HostPatch](w, r); ok {
		v, err := a.inv.PatchHost(r.Context(), actorOf(r), id(r), in)
		result(w, 200, v, err)
	}
}
func (a *api) deleteHost(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteHost(r.Context(), actorOf(r), id(r)))
}
func (a *api) restoreHost(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.RestoreHost(r.Context(), actorOf(r), id(r)))
}

func done(w http.ResponseWriter, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, http.StatusNoContent, nil)
}

func (a *api) addService(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Service](w, r); ok {
		v, err := a.inv.AddService(r.Context(), actorOf(r), id(r), in)
		if err == nil {
			a.icons.Queue(a.inv, v)
		}
		result(w, 201, v, err)
	}
}
func (a *api) updateService(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Service](w, r); ok {
		v, err := a.inv.UpdateService(r.Context(), actorOf(r), id(r), in)
		if err == nil {
			a.icons.Queue(a.inv, v)
		}
		result(w, 200, v, err)
	}
}
func (a *api) deleteService(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteService(r.Context(), actorOf(r), id(r)))
}
func (a *api) restoreService(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.RestoreService(r.Context(), actorOf(r), id(r)))
}

func (a *api) listProjects(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListProjects(r.Context())
	result(w, 200, v, err)
}
func (a *api) getProject(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.GetProject(r.Context(), id(r))
	result(w, 200, v, err)
}
func (a *api) createProject(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Project](w, r); ok {
		v, err := a.inv.CreateProject(r.Context(), actorOf(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) updateProject(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Project](w, r); ok {
		v, err := a.inv.UpdateProject(r.Context(), actorOf(r), id(r), in)
		result(w, 200, v, err)
	}
}
func (a *api) deleteProject(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteProject(r.Context(), actorOf(r), id(r)))
}
func (a *api) restoreProject(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.RestoreProject(r.Context(), actorOf(r), id(r)))
}

func (a *api) addProjectLink(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.ProjectLink](w, r); ok {
		v, err := a.inv.AddProjectLink(r.Context(), actorOf(r), projectID(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) deleteProjectLink(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteProjectLink(r.Context(), actorOf(r), id(r)))
}

func (a *api) addProjectEnv(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.ProjectEnv](w, r); ok {
		v, err := a.inv.AddProjectEnv(r.Context(), actorOf(r), projectID(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) deleteProjectEnv(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteProjectEnv(r.Context(), actorOf(r), id(r)))
}

func (a *api) addAppLink(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.AppLink](w, r); ok {
		v, err := a.inv.AddAppLink(r.Context(), actorOf(r), projectID(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) deleteAppLink(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteAppLink(r.Context(), actorOf(r), id(r)))
}

type projectHostIn struct {
	HostID int64  `json:"host_id"`
	Env    string `json:"env"`
}

func (a *api) linkProjectHost(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[projectHostIn](w, r)
	if !ok {
		return
	}
	done(w, a.inv.LinkProjectHost(r.Context(), actorOf(r), projectID(r), in.HostID, in.Env))
}

type projectServiceIn struct {
	ServiceID int64  `json:"service_id"`
	Env       string `json:"env"`
}

func (a *api) linkProjectService(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[projectServiceIn](w, r)
	if !ok {
		return
	}
	done(w, a.inv.LinkProjectService(r.Context(), actorOf(r), projectID(r), in.ServiceID, in.Env))
}

func (a *api) createConnection(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[inventory.Connection](w, r)
	if !ok {
		return
	}
	in.ProjectID = projectID(r)
	v, err := a.inv.CreateConnection(r.Context(), actorOf(r), in)
	result(w, 201, v, err)
}
func (a *api) deleteConnection(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteConnection(r.Context(), actorOf(r), id(r)))
}
func (a *api) usedBy(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.UsedBy(r.Context(), r.URL.Query().Get("target_kind"), queryInt(r, "target_id"))
	result(w, 200, v, err)
}

func (a *api) listLocations(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListLocations(r.Context())
	result(w, 200, v, err)
}
func (a *api) getLocation(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.GetLocation(r.Context(), id(r))
	result(w, 200, v, err)
}
func (a *api) createLocation(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Location](w, r); ok {
		v, err := a.inv.CreateLocation(r.Context(), actorOf(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) updateLocation(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Location](w, r); ok {
		v, err := a.inv.UpdateLocation(r.Context(), actorOf(r), id(r), in)
		result(w, 200, v, err)
	}
}
func (a *api) deleteLocation(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteLocation(r.Context(), actorOf(r), id(r)))
}
func (a *api) restoreLocation(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.RestoreLocation(r.Context(), actorOf(r), id(r)))
}

func (a *api) listTags(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListTags(r.Context())
	result(w, 200, v, err)
}
func (a *api) listGroups(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListGroups(r.Context())
	result(w, 200, v, err)
}

func (a *api) listAccess(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListAccess(r.Context(), r.URL.Query().Get("target_entity"), queryInt(r, "target_id"))
	result(w, 200, v, err)
}
func (a *api) createAccess(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Access](w, r); ok {
		v, err := a.inv.CreateAccess(r.Context(), actorOf(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) deleteAccess(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteAccess(r.Context(), actorOf(r), id(r)))
}

func (a *api) listCosts(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListCosts(r.Context())
	result(w, 200, v, err)
}
func (a *api) getCost(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.GetCost(r.Context(), id(r))
	result(w, 200, v, err)
}
func (a *api) createCost(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Cost](w, r); ok {
		v, err := a.inv.CreateCost(r.Context(), actorOf(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) updateCost(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Cost](w, r); ok {
		v, err := a.inv.UpdateCost(r.Context(), actorOf(r), id(r), in)
		result(w, 200, v, err)
	}
}
func (a *api) deleteCost(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteCost(r.Context(), actorOf(r), id(r)))
}
func (a *api) restoreCost(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.RestoreCost(r.Context(), actorOf(r), id(r)))
}

func (a *api) listDomains(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.ListDomains(r.Context())
	result(w, 200, v, err)
}
func (a *api) getDomain(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.GetDomain(r.Context(), id(r))
	result(w, 200, v, err)
}
func (a *api) createDomain(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Domain](w, r); ok {
		v, err := a.inv.CreateDomain(r.Context(), actorOf(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) updateDomain(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.Domain](w, r); ok {
		v, err := a.inv.UpdateDomain(r.Context(), actorOf(r), id(r), in)
		result(w, 200, v, err)
	}
}
func (a *api) deleteDomain(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteDomain(r.Context(), actorOf(r), id(r)))
}
func (a *api) restoreDomain(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.RestoreDomain(r.Context(), actorOf(r), id(r)))
}
func (a *api) addDomainRecord(w http.ResponseWriter, r *http.Request) {
	if in, ok := decode[inventory.DomainRecord](w, r); ok {
		v, err := a.inv.AddDomainRecord(r.Context(), actorOf(r), id(r), in)
		result(w, 201, v, err)
	}
}
func (a *api) deleteDomainRecord(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.DeleteDomainRecord(r.Context(), actorOf(r), id(r)))
}

func (a *api) collectFacts(w http.ResponseWriter, r *http.Request) {
	if a.facts == nil {
		reply(w, 422, map[string]string{"error": "SSH facts are not configured (set HS_SSH_KEY)"})
		return
	}
	h, err := a.inv.GetHost(r.Context(), id(r))
	if err != nil {
		fail(w, err)
		return
	}
	if len(h.Addresses) == 0 {
		reply(w, 422, map[string]string{"error": "host has no address"})
		return
	}
	f, err := a.facts.Collect(r.Context(), h.Addresses[0].Address)
	if err != nil {
		reply(w, 502, map[string]string{"error": err.Error()})
		return
	}
	f.HostID = h.ID
	if err := a.inv.SaveHostFacts(r.Context(), actorOf(r), *f); err != nil {
		fail(w, err)
		return
	}
	reply(w, 200, f)
}

func (a *api) freePort(w http.ResponseWriter, r *http.Request) {
	near := int(queryInt(r, "near"))
	if near == 0 {
		near = 3000
	}
	p, err := a.inv.FreePort(r.Context(), id(r), near)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, 200, map[string]int{"port": p})
}

func (a *api) search(w http.ResponseWriter, r *http.Request) {
	limit := int(queryInt(r, "limit"))
	v, err := a.inv.Search(r.Context(), r.URL.Query().Get("q"), limit)
	result(w, 200, v, err)
}

func (a *api) listAudit(w http.ResponseWriter, r *http.Request) {
	limit := int(queryInt(r, "limit"))
	v, err := a.inv.ListAudit(r.Context(), r.URL.Query().Get("entity"), queryInt(r, "id"), limit)
	result(w, 200, v, err)
}

func (a *api) listChangesets(w http.ResponseWriter, r *http.Request) {
	v, err := a.cs.List(r.Context(), r.URL.Query().Get("status"))
	result(w, 200, v, err)
}

func (a *api) createChangeset(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[struct {
		Title  string          `json:"title"`
		Source string          `json:"source"`
		Ops    []changesets.Op `json:"ops"`
	}](w, r)
	if !ok {
		return
	}
	if in.Source == "" {
		in.Source = "api"
	}
	v, err := a.cs.Create(r.Context(), actorOf(r), in.Title, in.Source, in.Ops)
	result(w, 201, v, err)
}

func (a *api) getChangeset(w http.ResponseWriter, r *http.Request) {
	v, err := a.cs.Get(r.Context(), id(r))
	result(w, 200, v, err)
}

func (a *api) applyChangeset(w http.ResponseWriter, r *http.Request) {
	v, err := a.cs.Apply(r.Context(), actorOf(r), id(r))
	result(w, 200, v, err)
}

func (a *api) rejectChangeset(w http.ResponseWriter, r *http.Request) {
	done(w, a.cs.Reject(r.Context(), actorOf(r), id(r)))
}

func (a *api) undoChangeset(w http.ResponseWriter, r *http.Request) {
	done(w, a.cs.Undo(r.Context(), actorOf(r), id(r)))
}

func (a *api) listDiscovery(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "new"
	}
	v, err := a.inv.ListDiscovered(r.Context(), status)
	result(w, 200, v, err)
}

func (a *api) discoveryStatus(w http.ResponseWriter, r *http.Request) {
	if a.scan == nil {
		reply(w, 200, discovery.Status{})
		return
	}
	reply(w, 200, a.scan.Status())
}

// scanDiscovery starts the same background runner the UI uses, so progress,
// configured ports and new-device alerts behave identically.
func (a *api) scanDiscovery(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[struct {
		Subnet string `json:"subnet,omitempty"`
		Ports  []int  `json:"ports,omitempty"`
	}](w, r)
	if !ok {
		return
	}
	if a.scan == nil {
		reply(w, 503, map[string]string{"error": "discovery runner is not configured"})
		return
	}
	var subnets []string
	if in.Subnet != "" {
		subnets = discovery.ParseSubnets(in.Subnet)
		if len(subnets) == 0 {
			reply(w, 422, map[string]string{"error": "subnet must be a CIDR like 192.168.1.0/24"})
			return
		}
	} else {
		subnets = a.scan.Subnets(r.Context())
	}
	if len(in.Ports) > discovery.MaxPorts {
		reply(w, 422, map[string]string{"error": "too many ports (max 64)"})
		return
	}
	for _, p := range in.Ports {
		if p < 1 || p > 65535 {
			reply(w, 422, map[string]string{"error": "ports must be 1–65535"})
			return
		}
	}
	if err := a.scan.StartWith(subnets, in.Ports, actorOf(r)); err != nil {
		if errors.Is(err, discovery.ErrBusy) {
			reply(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		fail(w, err)
		return
	}
	reply(w, http.StatusAccepted, a.scan.Status())
}

func (a *api) ignoreDiscovered(w http.ResponseWriter, r *http.Request) {
	done(w, a.inv.IgnoreDiscovered(r.Context(), actorOf(r), id(r)))
}

func (a *api) registerDiscovered(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[struct {
		Name string `json:"name"`
	}](w, r)
	if !ok {
		return
	}
	v, err := a.inv.RegisterDiscovered(r.Context(), actorOf(r), id(r), in.Name)
	result(w, 201, v, err)
}

func (a *api) listIntegrations(w http.ResponseWriter, r *http.Request) {
	if a.intg == nil {
		reply(w, 200, []any{})
		return
	}
	v, err := a.intg.List(r.Context())
	result(w, 200, v, err)
}

func (a *api) saveIntegration(w http.ResponseWriter, r *http.Request) {
	if a.intg == nil {
		fail(w, inventory.ErrNotFound)
		return
	}
	in, ok := decode[struct {
		Config  map[string]string `json:"config"`
		Secrets map[string]string `json:"secrets"`
	}](w, r)
	if !ok {
		return
	}
	values := map[string]string{}
	for k, v := range in.Config {
		values[k] = v
	}
	for k, v := range in.Secrets {
		values[k] = v
	}
	if err := a.intg.Save(r.Context(), actorOf(r), chi.URLParam(r, "id"), values); err != nil {
		fail(w, err)
		return
	}
	reply(w, 200, map[string]string{"status": "saved"})
}

func (a *api) testIntegration(w http.ResponseWriter, r *http.Request) {
	if a.intg == nil {
		fail(w, inventory.ErrNotFound)
		return
	}
	if err := a.intg.Test(r.Context(), chi.URLParam(r, "id")); err != nil {
		reply(w, 502, map[string]string{"error": err.Error()})
		return
	}
	reply(w, 200, map[string]string{"status": "ok"})
}

func (a *api) syncIntegration(w http.ResponseWriter, r *http.Request) {
	if a.intg == nil {
		fail(w, inventory.ErrNotFound)
		return
	}
	ops, err := a.intg.Sync(r.Context(), chi.URLParam(r, "id"))
	if err != nil && len(ops) == 0 {
		reply(w, 502, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{"ops": ops}
	if err != nil {
		out["warning"] = err.Error()
	}
	reply(w, 200, out)
}

func (a *api) listChecks(w http.ResponseWriter, r *http.Request) {
	v, err := a.chk.Store().List(r.Context(), r.URL.Query().Get("target_entity"), queryInt(r, "target_id"))
	result(w, 200, v, err)
}

func (a *api) createCheck(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[checks.Check](w, r)
	if !ok {
		return
	}
	v, err := a.chk.Store().Create(r.Context(), actorOf(r), in)
	result(w, 201, v, err)
}

func (a *api) runCheck(w http.ResponseWriter, r *http.Request) {
	res, err := a.chk.RunNow(r.Context(), id(r))
	if err != nil {
		fail(w, err)
		return
	}
	out := map[string]any{"ok": res.OK, "message": res.Message, "latency_ms": res.LatencyMS}
	if res.SSHStatus != "" {
		out["ssh_status"] = res.SSHStatus
	}
	if c, err := a.chk.Store().Get(r.Context(), id(r)); err == nil {
		out["state"] = c.State
	}
	reply(w, 200, out)
}

func (a *api) deleteCheck(w http.ResponseWriter, r *http.Request) {
	done(w, a.chk.Store().Delete(r.Context(), actorOf(r), id(r)))
}

func (a *api) problems(w http.ResponseWriter, r *http.Request) {
	v, err := a.chk.Store().ListProblems(r.Context(), int(queryInt(r, "limit")))
	result(w, 200, v, err)
}

func (a *api) export(w http.ResponseWriter, r *http.Request) {
	v, err := a.inv.Export(r.Context())
	result(w, 200, v, err)
}

func (a *api) importData(w http.ResponseWriter, r *http.Request) {
	in, ok := decode[inventory.ExportData](w, r)
	if !ok {
		return
	}
	apply := r.URL.Query().Get("apply") == "1" || r.URL.Query().Get("dry_run") == "false"
	v, err := a.inv.Import(r.Context(), actorOf(r), &in, r.URL.Query().Get("mode"), !apply)
	result(w, 200, v, err)
}
