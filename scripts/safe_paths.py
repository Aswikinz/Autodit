"""Constrain operational file access to an explicitly selected directory."""
from pathlib import Path


def confined_path(base: Path, path: str | Path) -> Path:
    """Resolve dot segments and symlinks before checking the directory boundary."""
    boundary = base.resolve()
    candidate = (boundary / path).resolve()
    if not candidate.is_relative_to(boundary) or candidate == boundary:
        raise ValueError("Path must identify a file inside the allowed directory")
    return candidate
