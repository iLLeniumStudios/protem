# Website

`protem.dev` is one GitHub Pages site: the landing page from `site/landing/` at `/`, and the docs built from `docs/` by MkDocs (Material theme, `mkdocs.yml`) at `/docs/`.

- `make site` builds everything into `_site/`. It needs Python 3 only: it creates a virtual environment in `bin/site-venv` from `site/docs/requirements.txt` and fetches Mermaid (checksum-verified) into `bin/`.
- `make site-serve` builds, then serves `_site/` on http://localhost:8000 (`SITE_PORT=9000` to change).
- The build is strict: a broken link or a page missing from `nav` in `mkdocs.yml` fails it.
- `site/docs/overrides/` holds the theme overrides, fonts and CSS; `site/docs/hooks.py` adds the brand files and points links to repository files outside the site at GitHub.
- `.github/workflows/pages.yml` builds on pull requests and deploys on pushes to `main`.
