# Concepts

## Hosts, services and projects

These three are the heart of the inventory, and the difference matters.

| | What it is | Example |
|---|---|---|
| **Host** | A machine: VM, LXC, bare metal or VPS. Has addresses, SSH logins, tags and a lifecycle. | `nas`, `app-1`, `vps-1` |
| **Service** | Something running *on* a host, with a port and optionally a public domain. Gets health checks. | Grafana on `monitor:3000`, Immich on `nas:2283` |
| **Project** | Something you *build or run* that can span several hosts and services, with environments and app links. | "Acme" with production and staging |

- The **Hosts** page lists machines. A host page shows its services, SSH posture, facts and logins.
- The **Services** page is a flat, searchable list of *every* service across all hosts — handy for
  "where is Grafana?" when you do not remember which machine.
- The **Projects** page groups the things that belong together and records what each project
  depends on (`used_by` answers *"what breaks if this host is down?"*).
- **Locations** say where a host lives: a Proxmox node, a VPS provider, a shelf.

## Lifecycle

Hosts are `planned`, `active`, `deprecated` or `decommissioned`. The dashboard's **Active / Inactive**
filter treats *down* and non-active hosts as inactive. Nothing is ever hard-deleted from the UI:
removing something is a **soft delete** you can restore, and every change is in the audit log.

## Checks and alerts

A check is a probe run on a schedule:

| Kind | Does |
|---|---|
| `http` | requests a URL, expects a status range (default 200–499) |
| `tcp` | opens a connection to a port |
| `tls` | reads the certificate and tracks days until expiry |
| `dns` | resolves a name |
| `ssh` | connects and reports whether the server accepts keys only or still accepts passwords |

Auto checks are created for services from their port and domain, and removed when those change.
A service with both a LAN address and a domain gets two, so you can tell a LAN outage from a broken
public route. Each check moves through `up → failing → down → recovering → up`; an alert fires when it
goes **down**, and again when it comes back after a real outage. Short blips shorter than the
flap guard (default 2 minutes) stay quiet.

Raw results are kept for 30 days and hourly roll-ups for a year, in the same SQLite file. The
**Alerts → History** tab lists incidents for the last 24 h, 7 or 30 days.

**Maintenance windows** silence alerts for one target or everything while checks keep running.

## Logins without passwords

An *access entry* records **how** you sign in somewhere — method (password, SSO, SSH key, token,
passkey), account name, 2FA type, and a **link to the item in your password manager**. It never
stores the secret. This is deliberate: it makes the inventory safe to share with an AI agent, which
can say "log in to Grafana as `admin` (item: …)" without ever seeing a password.

## Changesets: the review queue

Anything that is not you clicking a button goes through a changeset:

```
agent or integration  →  proposes ops  →  you review  →  apply (undo later)  or  reject
```

An op is a small statement such as *create host `printer` at 192.168.1.50* or *link service `Jellyfin`
to host `media`*. The page shows each op with the evidence the proposer gave. Applied changesets can
be undone from history. Plugin syncs use the same path, minus anything already in the inventory.

Two kinds of data are exceptions because they are observations, not decisions: **facts** (OS, uptime,
disk use, health warnings reported by SSH or an integration) are written directly.

## Facts and health warnings

A host page has a **Facts** card: OS, kernel, uptime, disk use, failed units, whether SSH accepts
passwords. They come from the read-only SSH collector (*Collect*) or from integrations (Proxmox,
Beszel, OpenMediaVault, Scrutiny…). When a source reports a problem — a degraded pool, a failing disk,
a UPS on battery — it shows on the host and under **Alerts** as *needs a look*.

## Discovery

Discovery sweeps the subnets you list for a configurable set of TCP ports (a plain connect — no
logins, no payloads), then identifies what answered using reverse DNS, HTTP titles, banners and
favicon hashes. New addresses land in an **inbox**; you *Register* one as a host (with services guessed
from its ports) or *Ignore* it. Scans run in the background; scheduled scans are optional.
Edit the ports in **Discovery → Scan settings** or **Settings → Scanning**.

## Domains and costs

- **Domains** track registrar, expiry, auto-renew, price and DNS records. *Look up* fills registrar and
  expiry from public RDAP data; renewal reminders go out at 30, 7 and 1 days.
- **Costs** are monthly, yearly, one-time or estimated, in any currency; totals use daily ECB rates or
  your manual rates. The **Electricity** plugin turns watts per host (set by hand or read from Home
  Assistant, Shelly or Tasmota) into a monthly cost.

## Favourites

Star a tile on the dashboard to add it to the collapsed **Favourites** section above the tiles.
