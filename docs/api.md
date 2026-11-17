# REST API & exports

## Authentication

`/api/v1/*` and `/mcp` always require a token, independent of the UI login:

```bash
curl -H "Authorization: Bearer $HS_TOKEN" http://localhost:8080/api/v1/hosts
```

Tokens come from `homeservices token create` or **Settings → MCP & AI access**, with scope `read`, `propose` or
`write` (see [AI agents](ai-agents.md#scopes)). A request from a signed-in browser session, or same-origin when
`HS_AUTH=none`, is also accepted so the UI can use the same endpoints.

The API uses numeric ids, not slugs: `/api/v1/projects/12`.

## Endpoints

Collections support the obvious verbs: `GET` list/read, `POST` create, `PUT`/`PATCH` update, `DELETE` soft-delete
and `POST …/restore`.

| Area | Paths |
|---|---|
| Hosts | `/hosts`, `/hosts/{id}`, `/hosts/{id}/services`, `/hosts/{id}/facts` (collect over SSH) |
| Services | `/services/{id}` |
| Projects | `/projects`, `/projects/{id}`, `…/envs`, `…/app-links`, `…/links`, `…/hosts`, `…/services`, `…/connections` |
| Locations | `/locations` |
| Access (logins) | `/access` |
| Domains | `/domains`, `/domains/{id}/records` |
| Costs | `/costs` |
| Checks | `/checks`, `/checks/{id}/run`, `/problems` |
| Discovery | `/discovery`, `/discovery/scan`, `/discovery/status`, `…/{id}/register`, `…/{id}/ignore` |
| Changesets | `/changesets`, `…/{id}/apply`, `…/{id}/reject`, `…/{id}/undo` |
| Integrations | `/integrations`, `…/{id}/test`, `…/{id}/sync` |
| Audit | `/audit` |
| Backup / restore | `GET /export`, `POST /import?apply=1&mode=merge` |

Errors are JSON `{"error": "…"}` with a sensible status (`400` invalid, `404` not found, `403` scope too low).
Writes are validated by the same store the UI uses, so rules (unique names, valid kinds) are identical.

### Example: propose a change

```bash
curl -X POST http://localhost:8080/api/v1/changesets \
  -H "Authorization: Bearer $HS_TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"Add Jellyfin","source":"script","ops":[
        {"action":"create","entity":"host","ref":"media","data":{"kind":"lxc","address":"192.168.1.77"}},
        {"action":"create","entity":"service","ref":"Jellyfin","data":{"host":"media","port":"8096","protocol":"http"}}]}'
```

## Exports

The inventory can generate files for other tools. They are built fresh on every request and contain **no passwords,
keys or tokens**. Open them in the browser under `/export/<name>` (**Settings → Integrations → Export** has *View* and
*Download* buttons) or fetch them with a read token under `/api/v1/export/<name>` on a schedule.

| File | Use it for |
|---|---|
| `homepage.yaml` | a [Homepage](https://gethomepage.dev) `services.yaml` grouped by location, with links and icons |
| `ansible.yaml` | an Ansible inventory with groups by location, tag and kind, plus `ansible_host`, user and port |
| `prometheus.json` | Prometheus `file_sd` targets (add `?port=9100`, or `?all=1` for every host) |
| `netbox-ips.csv` | a CSV of IP addresses ready for NetBox's bulk import |
| `/export/ssh-config` | an OpenSSH client config for every host with SSH access |
| `/export/inventory.json` | the whole inventory as JSON (the format `import` reads) |

```bash
# refresh a Homepage config from the inventory every night
curl -fsS -H "Authorization: Bearer $HS_TOKEN" \
  http://homeservices.lan/api/v1/export/homepage.yaml -o /opt/homepage/config/services.yaml
```

Exports never replace your tools' own config by themselves — you choose where the file goes.

## Plain-text context

`/inventory.md` (with a token) returns the whole inventory as Markdown, and `/hosts/{id}/context.md` one host — handy to
paste into any AI chat or feed to an agent that cannot use MCP.

## Backup and import from the CLI

```bash
homeservices backup                     # database + encrypted secrets, pruned to the last 14
homeservices import file.json           # dry run: shows what would change
homeservices import -apply file.json    # merge (match by name / slug); -mode replace wipes first
```
