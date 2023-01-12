from asyncio import PriorityQueue
from random import randint
import json
from typing import Any, Dict, Generic, List, Optional, TypeVar
from fastapi import APIRouter

from pixivpy_async import AppPixivAPI

from app.models.image import Image

router = APIRouter()

class Config:
    def __getattr__(self, name):
        with open("config.json", "r") as f:
            config = json.load(f)
        return config.get("pixiv").get(name, None)

    def __setattr__(self, _name: str, _value: Any) -> None:
        with open("config.json", "r") as f:
            config = json.load(f)
        config["pixiv"][_name] = _value
        with open("config.json", "w") as f:
            json.dump(config, f, indent=2)

config = Config()

api = AppPixivAPI(proxy=config.proxy)

json_result: Optional[Dict[str, Any]] = None

def should_skip(tags: List[Dict[str, str]]) -> bool:
    """filter r18 images (based on tags)
    """
    skip = False
    blacklist = config.blacklist if config.blacklist else []
    for t in tags:
        if config.r18 == False and t.get("name", "") == "R-18":
            skip = True
            break
        if len(blacklist) and t.get("name", "") in blacklist:
            skip = True
            break
    return skip

T = TypeVar("T")
class PriorityEntry(Generic[T]):
    def __init__(self, priority: int, data: T):
        self.priority = priority
        self.data = data
    def __lt__(self, other):
        return self.priority < other.priority


queue: PriorityQueue[PriorityEntry[Image]] = PriorityQueue()


def _to_image(url: str, data: Dict[str, Any]) -> Image:
    return Image(
        url=url,
        title=data["title"],
        author=data["user"]["name"],
        data=data,
    )


@router.get("/pixiv", response_model=Image)
async def pixiv() -> Image:
    if queue.empty():
        if config.refresh_token is not None:
            await api.login(refresh_token=config.refresh_token)
        if api.refresh_token is None:
            await api.login_web()
        config.refresh_token = api.refresh_token
        recommend_base_illusts = config.recommend_base_illusts
        global json_result
        next_qs = None
        if json_result is not None and json_result.get("next_url", None):
            next_qs = api.parse_qs(json_result.get("next_url"))
        if next_qs is not None:
            json_result = await api.illust_recommended(**next_qs)
        if recommend_base_illusts is None:
            json_result = await api.illust_recommended(content_type="illust")
        else:
            json_result = await api.illust_recommended(content_type="illust", bookmark_illust_ids=recommend_base_illusts)
        for i in json_result["illusts"]:
            # filter r18 images (based on tags)
            tags = i.get("tags", [])
            if should_skip(tags):
                continue

            url = i["meta_single_page"].get("original_image_url", None)
            if url is None:
                for j in i["meta_pages"]:
                    queue.put_nowait(PriorityEntry(randint(0, 100), _to_image(
                        j["image_urls"]["original"].replace("i.pximg.net", "i.pixiv.re"),
                        i,
                    )))
            else:
                queue.put_nowait(PriorityEntry(randint(0, 100), _to_image(url.replace("i.pximg.net", "i.pixiv.re"), i)))
    return queue.get_nowait().data
