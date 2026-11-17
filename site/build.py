#!/usr/bin/env python3
"""Builds the GitHub Pages site into ./_site from site/ (landing page, CSS) and docs/*.md.

    pip install markdown
    python3 site/build.py

The markdown in docs/ stays plain (it renders fine on GitHub too); this script only
wraps it in the site layout and rewrites links between pages.
"""
import html
import re
import shutil
from pathlib import Path

import markdown

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "_site"
REPO = "https://github.com/dawidof/home-services"

# (file, nav title, one-line description)
PAGES = [
    ("getting-started", "Getting started", "Install, first run, and adding your first hosts and services."),
    ("concepts", "Concepts", "Hosts vs services vs projects, checks, changesets and passwordless logins."),
    ("integrations", "Integrations", "31 read-only plugins: Proxmox, Caddy, UniFi, MikroTik, OpenMediaVault and more."),
    ("ai-agents", "AI agents (MCP)", "Tokens, scopes, tools and example prompts for Claude Code and other agents."),
    ("api", "REST API & exports", "Endpoints and generated Homepage, Ansible, Prometheus and NetBox files."),
    ("configuration", "Configuration", "Environment variables, the command line and settings pages."),
    ("deployment", "Deployment", "systemd, Docker, reverse proxy, updates with rollback, backups."),
    ("architecture", "Architecture", "Packages, ground rules, testing and how to add an integration."),
    ("faq", "FAQ & troubleshooting", "Common questions and fixes."),
]
TITLES = {p[0]: p[1] for p in PAGES}


def rewrite_links(body: str) -> str:
    # page.md#anchor -> page.html#anchor ; images/ -> ../images/
    body = re.sub(r'href="([a-z0-9-]+)\.md(#[^"]*)?"', lambda m: f'href="{m.group(1)}.html{m.group(2) or ""}"', body)
    body = body.replace('src="images/', 'src="../images/')
    # open external links in the same tab, but mark them
    body = re.sub(r'<a href="(https?://[^"]+)"', r'<a class="ext" href="\1"', body)
    return body


def nav_html(active: str, prefix: str) -> str:
    items = []
    for slug, title, _ in PAGES:
        cls = ' class="active"' if slug == active else ""
        items.append(f'<a{cls} href="{prefix}{slug}.html">{html.escape(title)}</a>')
    return "\n".join(items)


def main() -> None:
    if OUT.exists():
        shutil.rmtree(OUT)
    (OUT / "docs").mkdir(parents=True)

    shutil.copytree(ROOT / "docs" / "images", OUT / "images")
    shutil.copy(ROOT / "assets" / "favicon.svg", OUT / "favicon.svg")
    shutil.copy(ROOT / "site" / "style.css", OUT / "style.css")
    (OUT / ".nojekyll").write_text("")

    layout = (ROOT / "site" / "doc.html").read_text()
    for i, (slug, title, desc) in enumerate(PAGES):
        src = (ROOT / "docs" / f"{slug}.md").read_text()
        md = markdown.Markdown(extensions=["fenced_code", "tables", "toc", "sane_lists", "attr_list"],
                               extension_configs={"toc": {"permalink": "#", "toc_depth": "2-3"}})
        body = rewrite_links(md.convert(src))
        # drop the leading <h1>: the layout prints its own
        body = re.sub(r"^\s*<h1[^>]*>.*?</h1>\s*", "", body, count=1, flags=re.S)
        prev_link = next_link = ""
        if i > 0:
            p = PAGES[i - 1]
            prev_link = f'<a class="prev" href="{p[0]}.html"><small>Previous</small>{html.escape(p[1])}</a>'
        if i < len(PAGES) - 1:
            n = PAGES[i + 1]
            next_link = f'<a class="next" href="{n[0]}.html"><small>Next</small>{html.escape(n[1])}</a>'
        page = (layout
                .replace("{{title}}", html.escape(title))
                .replace("{{description}}", html.escape(desc))
                .replace("{{nav}}", nav_html(slug, ""))
                .replace("{{toc}}", md.toc)
                .replace("{{body}}", body)
                .replace("{{prev}}", prev_link)
                .replace("{{next}}", next_link)
                .replace("{{slug}}", slug)
                .replace("{{repo}}", REPO))
        (OUT / "docs" / f"{slug}.html").write_text(page)

    index = (ROOT / "site" / "index.html").read_text().replace("{{repo}}", REPO)
    (OUT / "index.html").write_text(index)
    (OUT / "404.html").write_text((ROOT / "site" / "404.html").read_text())
    print(f"built {len(PAGES)} docs pages into {OUT}")


if __name__ == "__main__":
    main()
