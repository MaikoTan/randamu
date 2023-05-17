from typing import List, Optional
from pydantic import BaseModel


class Image(BaseModel):
    url: str
    fallback_urls: List[str] = []
    title: str = ""
    page_url: Optional[str] = None
    author: str = ""
    author_url: Optional[str] = None
    data_url: Optional[str] = None
    data: Optional[dict] = None
