"""MkDocs hooks for the Protem docs site (registered in mkdocs.yml)."""

import re
from pathlib import Path

from mkdocs.structure.files import File

BRAND = {
    "assets/brand/protem-symbol-white.svg": "brand/logo/protem-symbol-white.svg",
    "assets/brand/favicon.svg": "brand/web/favicon.svg",
    "assets/brand/favicon.ico": "brand/web/favicon.ico",
    "assets/brand/apple-touch-icon.png": "brand/web/apple-touch-icon.png",
}

LINK = re.compile(r"\]\(([^()\s#]+)(#[^()\s]*)?\)")


def _root(config):
    return Path(config.config_file_path).parent.resolve()


def on_files(files, config):
    root = _root(config)
    for dest, src in BRAND.items():
        files.append(File.generated(config, dest, abs_src_path=str(root / src)))
    return files


def on_page_markdown(markdown, page, config, files):
    """Point links at repository files that are not on the site (outside docs/, or excluded) to GitHub."""
    root, docs = _root(config), Path(config.docs_dir).resolve()
    here = Path(page.file.abs_src_path).parent

    def rewrite(m):
        target_text, fragment = m[1], m[2] or ""
        if "://" in target_text or target_text.startswith(("/", "mailto:")):
            return m[0]
        target = (here / target_text).resolve()
        if not target.is_file() or not target.is_relative_to(root):
            return m[0]
        if target.is_relative_to(docs):
            file = files.get_file_from_path(target.relative_to(docs).as_posix())
            if file is not None and not file.inclusion.is_excluded():
                return m[0]
        return f"]({config.repo_url}/blob/main/{target.relative_to(root).as_posix()}{fragment})"

    return LINK.sub(rewrite, markdown)
