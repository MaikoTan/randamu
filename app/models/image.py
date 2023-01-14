from typing import Any, Dict, Optional
from pydantic import BaseModel


class Image(BaseModel):
    url: str
    title: str = ""
    page_url: Optional[str] = None
    author: str = ""
    author_url: Optional[str] = None
    data: Optional[dict] = None
