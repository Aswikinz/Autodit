"""Regression tests for deployment and coverage file boundaries."""
from pathlib import Path
import tempfile
import unittest

from safe_paths import confined_path


class ConfinedPathTests(unittest.TestCase):
    def test_inside_and_absolute_inside(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            self.assertEqual(confined_path(root, "reports/coverage.out"), root / "reports/coverage.out")
            self.assertEqual(confined_path(root, root / "coverage.out"), root / "coverage.out")

    def test_parent_absolute_and_prefix_sibling_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            for candidate in ("..", "../private.txt", root.parent / "private.txt", Path(str(root) + "-sibling/private.txt"), "."):
                with self.subTest(candidate=str(candidate)), self.assertRaises(ValueError):
                    confined_path(root, candidate)

    def test_symlink_outside_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            allowed = root / "allowed"
            allowed.mkdir()
            secret = root / "secret.txt"
            secret.write_text("private")
            try:
                (allowed / "escape.txt").symlink_to(secret)
            except OSError as error:
                self.skipTest(f"symlink creation unavailable: {error}")
            with self.assertRaises(ValueError):
                confined_path(allowed, "escape.txt")


if __name__ == "__main__":
    unittest.main()
