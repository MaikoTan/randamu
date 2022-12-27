from typing import Optional

from pydantic import BaseModel


class LoliconConfig(BaseModel):
    r18 = 0
    excludeAI = True
    dateAfter: Optional[int]
    dateBefore: Optional[int]

class RandamuConfig(BaseModel):
    interval = 30
    service = "lolicon"
    lolicon: Optional[LoliconConfig]
