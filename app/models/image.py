from typing import Any, Dict, Optional
from pydantic import BaseModel


class Image(BaseModel):
    url: str
    title: str = ""
    author: str = ""
    data: Optional[dict] = None
