import json
from typing import Any, Dict, Optional


class Config(object):
    path: Optional[str] = None

    def __init__(self, path: Optional[str] = None):
        super().__setattr__("path", path)

    @property
    def root_config(self):
        return self.read_config()

    def read_config(self) -> Dict[str, Any]:
        with open("config.json", "r") as f:
            config = json.load(f)
        return config

    def save_config(self, config: Dict[str, Any]):
        with open("config.json", "w") as f:
            json.dump(config, f, indent=2, ensure_ascii=False)

    def __getattr__(self, name):
        config = self.read_config()
        if self.path is None:
            return config.get(name, None)
        else:
            return config.get(self.path, {}).get(name, None)

    def __setattr__(self, _name: str, _value: Any) -> None:
        if _name == "path":
            return

        config = self.read_config()
        if self.path is None:
            config[_name] = _value
        else:
            config[self.path][_name] = _value

        self.save_config(config)
