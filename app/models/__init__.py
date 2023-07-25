from typing import Optional

from pydantic import BaseModel


class LoliconConfig(BaseModel):
    r18: int = 0
    excludeAI: bool = True
    dateAfter: Optional[int]
    dateBefore: Optional[int]


class RandamuConfig(BaseModel):
    interval: int = 30
    service: str = "lolicon"
    lolicon: Optional[LoliconConfig]
