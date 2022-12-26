from typing import Optional

from fastapi import APIRouter
import requests

router = APIRouter()

@router.get("/lolicon")
def lolicon(tag=None, r18=0, num=1, excludeAI=True, dateAfter: Optional[int]=None, dateBefore: Optional[int]=None):
    payload = {
        "r18": r18,
        "num": num,
        "excludeAI": excludeAI,
    }
    if tag is not None:
        payload["tag"] = [tag]
    if dateAfter is not None:
        payload["dateAfter"] = dateAfter
    if dateBefore is not None:
        payload["dateBefore"] = dateBefore
    r = requests.post("https://api.lolicon.app/setu/v2", json=payload)
    j = r.json()
    i = j["data"][0]
    url: str = i["urls"]["original"]
    return { "url": url }
