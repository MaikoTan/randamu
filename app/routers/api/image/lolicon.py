from typing import Optional
import re

from fastapi import APIRouter
import requests

from app.models.image import Image

router = APIRouter()


@router.get("/lolicon", response_model=Image)
def lolicon(
    tag=None,
    r18=0,
    num=1,
    excludeAI=True,
    dateAfter: Optional[int] = None,
    dateBefore: Optional[int] = None,
) -> Image:
    payload = {
        "r18": r18,
        "num": num,
        "excludeAI": excludeAI,
        "proxy": "null",
        "size": ["regular", "original"],
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
    url: str = i["urls"]["regular"]
    if not url:
        return lolicon(tag, r18, num, excludeAI, dateAfter, dateBefore)
    return Image(
        url=re.sub(r"^https://i.pximg.net", "/api/image/pixiv/proxy", url),
        title=i.get("title", ""),
        author=i.get("author", ""),
        page_url=f'https://pixiv.net/i/{i.get("pid")}',
        author_url=f'https://pixiv.net/u/{i.get("uid")}',
        data=i,
    )
