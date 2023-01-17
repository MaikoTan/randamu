from asyncio import PriorityQueue
from random import randint
import json
from typing import Any, Dict, Generic, Iterable, List, Optional, TypeVar
from fastapi import APIRouter

from pixivpy_async import AppPixivAPI

from app.models.image import Image
from app.config import Config

router = APIRouter()

config = Config("pixiv")
api = AppPixivAPI(proxy=config.proxy)

json_result: Optional[Dict[str, Any]] = None

def should_skip(tags: Iterable[str], data: Dict[str, Any]) -> bool:
    """filter r18 images (based on tags and meta informations)
    """
    skip = False
    # `illust_ai_type == 2`` means that this is an AI-generated work
    if config.excludeAI and data.get("illust_ai_type", None) == 2:
        return True
    # `x_restrict == 2` means that this is an R18G work
    if config.r18g == False and data.get("x_restrict", 0) > 1:
        return True
    # `x_rescrtict == 1` means that this is an R18 work
    if config.r18 == False and data.get("x_restrict", 0) == 1:
        return True
    # filter blacklisted tags
    blacklist = config.blacklist if config.blacklist else []
    for t in tags:
        if len(blacklist) and t in blacklist:
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
        page_url=f'https://pixiv.net/i/{data.get("id")}',
        author_url=f'https://pixiv.net/u/{data.get("user", {}).get("id")}',
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
        next_url = config.next_url

        if next_url is not None:
            next_qs = api.parse_qs(next_url)
            if next_qs.get("word") is None:
                next_qs["word"] = config.tag
            json_result = await api.search_illust(**next_qs)
        elif recommend_base_illusts is not None:
            json_result = await api.illust_recommended(content_type="illust", bookmark_illust_ids=recommend_base_illusts)
        elif config.tag is not None:
            json_result = await api.search_illust(config.tag, sort="popular_desc")
        else:
            json_result = await api.illust_recommended(content_type="illust")

        if json_result is not None and json_result.get("next_url", None):
            config.next_url = json_result.get("next_url")

        for i in json_result["illusts"]:
            # filter r18 images (based on tags)
            tags = i.get("tags", [])
            if should_skip(map(lambda x: x.get("name", None), tags), i):
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
