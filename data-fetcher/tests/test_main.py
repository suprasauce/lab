import os
import unittest
from unittest.mock import patch

from main import main


class ConfigurationTests(unittest.TestCase):
    def test_database_url_comes_only_from_os(self):
        for environment in ({}, {"DATABASE_URL": "host=os-database"}):
            with self.subTest(environment=environment), patch.dict(os.environ, environment, clear=True), \
                    patch("main.dotenv_values", return_value={
                        "DATABASE_URL": "host=dotenv-database",
                        "BREEZE_API_KEY": "test-key",
                    }), patch("sys.argv", ["main.py"]), \
                    patch("pipelines.underlying.fetch_underlying") as fetch:
                def check_configuration(*args):
                    self.assertEqual(os.environ.get("DATABASE_URL"), environment.get("DATABASE_URL"))
                    self.assertEqual(os.environ["BREEZE_API_KEY"], "test-key")
                fetch.side_effect = check_configuration
                self.assertEqual(main(), 0)
                fetch.assert_called_once()


if __name__ == "__main__":
    unittest.main()
