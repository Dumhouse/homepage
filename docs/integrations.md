# Integrations

Integrations are **optional, read-only plugins**. Each one reads from a system you already run and turns what it
finds into a **changeset** you review on the *AI changes* page — nothing is added to your inventory until you apply it.
Facts such as OS, uptime and health warnings are the one exception: they are observations, so they are written directly.

Open **Settings → Integrations**, pick a plugin, fill in the fields, then **Test** (checks the connection) and
**Sync now**. The **Connected** row at the top lists every plugin you have set up and whether its last test worked.

## Good to know

- **Secrets are encrypted.** Tokens and passwords go into `data/secrets.enc` (AES-GCM) using `HS_MASTER_KEY`.
  Without that variable they are kept in memory only and lost on restart. They are never stored in SQLite and
  never shown again after saving (the field says *saved — leave blank to keep*).
- **Use read-only accounts** wherever the system lets you. Every plugin only reads.
- **Two of the same?** Under a plugin's settings, press **Add another …** (for example a second Proxmox server).
  Each copy has its own name, address and credentials, and **Remove this copy** deletes it.
- **Self-signed certificates** are common on a LAN; plugins that talk HTTPS have an *Accept self-signed certificate*
  switch, on by default.
- **Suggestions.** The top of the page recommends plugins based on what is already in your inventory (for
  example, Proxmox when a service on port 8006 is documented).
- Sync is **idempotent**: items already in the inventory are left out of the changeset.
- **Auto-sync.** Each importing plugin has an *Auto-sync* choice: off (the default), every 6 hours, daily or weekly.
  New items still arrive as a changeset for you to review, and while one from that plugin is waiting, the next
  sync is skipped so proposals never pile up.


## All 31 plugins

### Servers & containers

| Plugin | What it does |
|---|---|
| **Proxmox** (`proxmox`) | Read nodes, VMs and LXC containers, and propose host records. |
| **Docker** (`docker`) | Read running containers over SSH and propose a service per published port. |

### Network & DNS

| Plugin | What it does |
|---|---|
| **Caddy** (`caddy`) | Read hostnames and upstreams from the Caddy admin API on the edge box. |
| **Cloudflare** (`cloudflare`) | Read zones and DNS records; propose domains and record entries. |
| **UniFi Network** (`unifi`) | Import UniFi gateways, switches and access points as hosts, and fill in the LAN address of documented hosts from the client list. |
| **OPNsense** (`opnsense`) | Import the firewall as a host and fill in LAN addresses of documented hosts from its DHCP leases (ISC DHCP, Kea or Dnsmasq). |
| **MikroTik RouterOS** (`mikrotik`) | Import the router (board, RouterOS version, uptime) and fill in LAN addresses of documented hosts from its DHCP leases. RouterOS 7 REST API. |
| **OpenWrt** (`openwrt`) | Import the router (model, OpenWrt release, uptime) and fill in LAN addresses of documented hosts from its DHCP leases, over ubus. |
| **Pi-hole** (`pihole`) | Read Pi-hole's local DNS records and DHCP leases (v6 API) to fill in LAN addresses of documented hosts. |
| **AdGuard Home** (`adguard`) | Read AdGuard Home's DNS rewrites and DHCP leases to fill in LAN addresses of documented hosts. |
| **Nginx Proxy Manager** (`npm`) | Import proxy hosts as service links: each domain is attached to the service at its forward host and port. |
| **Traefik** (`traefik`) | Import HTTP routers as service links: each Host(`…`) rule is attached to the service behind it. |

### Cloud & VPS

| Plugin | What it does |
|---|---|
| **Hetzner Cloud** (`hetzner`) | Import cloud servers with their public and private addresses, type and location, and propose each server's monthly price as a cost. |
| **DigitalOcean** (`digitalocean`) | Import droplets with their public and private addresses, size and region, and propose each droplet's monthly price as a cost. |
| **OVHcloud** (`ovh`) | Import VPS and dedicated servers with their addresses, model and zone. |

### VPN

| Plugin | What it does |
|---|---|
| **Tailscale** (`tailscale`) | Add each machine's Tailscale address (100.x) to the matching host, so SSH and links work from anywhere. Optionally add tailnet devices that aren't documented yet. |
| **Headscale** (`headscale`) | Self-hosted Tailscale control server: add each node's mesh address to the matching host. |

### Metrics

| Plugin | What it does |
|---|---|
| **Prometheus** (`prometheus`) | Read node_exporter metrics: OS, kernel, root disk use and uptime of every scraped machine become host facts; unknown machines are proposed as hosts. |
| **Netdata** (`netdata`) | Read OS, kernel, root disk use and uptime from Netdata agents. Point it at a parent to get all its children in one go. |
| **Beszel** (`beszel`) | Read every system Beszel monitors: hostname, kernel, disk use and uptime become host facts; unknown systems are proposed as hosts. |
| **Glances** (`glances`) | Read OS, kernel, root disk use and uptime from Glances web servers (glances -w) on your machines. |

### Storage & hardware

| Plugin | What it does |
|---|---|
| **TrueNAS** (`truenas`) | Import a TrueNAS box: version, RAM, pools and their health, and the sharing services it runs (SMB, NFS, iSCSI, SSH). |
| **Synology DSM** (`synology`) | Import a Synology NAS: model, DSM version, RAM, volumes with their use and health. |
| **OpenMediaVault** (`omv`) | Import an OpenMediaVault NAS: version, uptime, RAM, how full each filesystem is, and SMART health. A failing disk or a nearly full filesystem shows as a warning on the host. |
| **NUT (UPS)** (`nut`) | Read your UPSes from a Network UPS Tools server: model, battery, load and runtime. A UPS on battery or with a low battery shows as a warning. |
| **IPMI / Redfish** (`redfish`) | Read a server's management controller (iDRAC, iLO, Supermicro, XClarity, OpenBMC) over Redfish: model, CPUs, RAM, power state, current power draw and hardware health. |

### Disk health

| Plugin | What it does |
|---|---|
| **Scrutiny** (`scrutiny`) | Read SMART disk health from Scrutiny. A failing disk shows as a warning on its host (Needs a look); healthy hosts get the warning cleared. |

### Power

| Plugin | What it does |
|---|---|
| **Electricity** (`power`) | What your hosts draw from the wall and what it costs per month. Set watts per host by hand, or read them live from Home Assistant, a Shelly or a Tasmota plug (host page → Power). |

### Notifications

| Plugin | What it does |
|---|---|
| **ntfy** (`ntfy`) | Push notifications to your phone through an ntfy topic (ntfy.sh or your own server). |
| **Gotify** (`gotify`) | Push notifications through a self-hosted Gotify server. |
| **Webhook** (`webhook`) | POST every event as JSON to a URL — Home Assistant, n8n, a script of your own. |

## Setup notes

What to create on the other side, for the plugins people ask about most. Every plugin's fields have inline help in the UI.

### Proxmox

**Create a read-only token** — Datacenter → Permissions → API Tokens → *Add*. Pick a user (an existing
one is fine), give it a name, and **untick Privilege Separation** (or give the token its own `PVEAuditor`
role on path `/`). Copy the secret once. In Home Services: API URL `https://<pve-ip>:8006`, Token id
`user@realm!tokenname`, the secret, and leave *Ignore TLS errors* on for the default self-signed certificate.
It reads nodes, VMs and containers and proposes them as hosts with their addresses.

### Caddy

Reads the Caddy **admin API** (`GET /config/`). By default it listens on `localhost:2019` on the proxy
machine only — which is the safe default. Either run Home Services on that machine, or expose the admin
endpoint only to Home Services (the admin API can reconfigure Caddy, so never expose it to the whole LAN).
Routes become services linked to their upstream host and port.

### MikroTik RouterOS

Needs **RouterOS 7** (REST API). Enable `www` or `www-ssl` under *IP → Services*, then add a read-only
user:

```
/user group add name=hs-ro policy=read,api,rest-api
/user add name=hs group=hs-ro password=<strong password>
```

URL `https://<router-ip>` (or `http://` if you use `www`). It imports the router and fills LAN addresses
for hosts you already documented from its DHCP leases; unknown leases are proposed as new devices.

### OpenMediaVault

Uses the same JSON-RPC endpoint (`/rpc.php`) as the OpenMediaVault web UI. Create a web-UI user with a
read-only role (or use `admin`), then enter the UI address, user and password. You get the OMV version, uptime,
RAM and how full each filesystem is, plus SMART status; a failing disk or a filesystem above 95% shows as a
warning on the host.

### ntfy

Topic URL such as `https://ntfy.example.com/homelab` (or `ntfy.sh/your-secret-topic`). If your server
requires authentication (HTTP 403 on test), create a token with `ntfy token add --user=<name>` on the server
and paste it as *Access token*. Subscribe to the same topic in the ntfy phone app.

### Cloudflare

Create an API token with **Zone → Zone → Read** and **Zone → DNS → Read**. Zones become domains and
records become DNS records, matched against your hosts.

### Tailscale

Admin console → Settings → Keys → *Generate access token*. It adds each machine's `100.x` address to the
host of the same name; turn on *Add missing devices* to also propose hosts for unknown devices.

### Docker

Reads running containers over SSH (`docker ps`) and proposes one service per published port. Give it a user in the
`docker` group and a private key; the key is stored encrypted and only used for read-only commands.

### Everything else

Open the plugin in the UI: each field explains where to find the value (for example *Cloud console → your project →
Security → API tokens* for Hetzner). Fields marked with an asterisk are required.

## Exports (the other direction)

Home Services can also **generate** files for other tools from the inventory — see [REST API & exports](api.md#exports):
a Homepage `services.yaml`, an Ansible inventory, Prometheus file-SD targets and a NetBox IP CSV.

## Writing your own

A plugin is a Go file with a field list, a `Test` and a `Sync` that returns proposals. See
[Architecture → Adding an integration](architecture.md#adding-an-integration).
