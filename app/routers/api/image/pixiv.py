from asyncio import PriorityQueue
from random import randint
import re

from typing import Any, Dict, Generic, Iterable, Optional, TypeVar
from fastapi import APIRouter
from fastapi.responses import StreamingResponse
import requests

from aiohttp import ClientSession
from pixivpy_async import AppPixivAPI

from app.models.image import Image
from app.config import Config

router = APIRouter()

config = Config("pixiv")
api = AppPixivAPI(proxy=config.proxy)

json_result: Optional[Dict[str, Any]] = None


def should_skip(tags: Iterable[str], data: Dict[str, Any]) -> bool:
    """filter r18 images (based on tags and meta informations)"""
    skip = False
    # `illust_ai_type == 2`` means that this is an AI-generated work
    if config.excludeAI and data.get("illust_ai_type", None) == 2:
        return True
    # `x_restrict == 2` means that this is an R18G work
    if config.r18g is False and data.get("x_restrict", 0) > 1:
        return True
    # `x_rescrtict == 1` means that this is an R18 work
    if config.r18 is False and data.get("x_restrict", 0) == 1:
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
        url=re.sub(r"^https://i.pximg.net", "/api/image/pixiv/proxy", url),
        title=data["title"],
        author=data["user"]["name"],
        page_url=f'https://pixiv.net/i/{data.get("id")}',
        author_url=f'https://pixiv.net/u/{data.get("user", {}).get("id")}',
        # data=data,
        pixiv_id=data.get("id"),
    )


@router.get("/pixiv", response_model=Image)
async def pixiv() -> Image:
    while not queue.empty():
        data = queue.get_nowait().data
        if data is None:
            return await pixiv()
        return data

    if api.refresh_token is None:
        if config.refresh_token is not None:
            await api.login(refresh_token=config.refresh_token)
        if api.refresh_token is None:
            await api.login_web()
        config.refresh_token = api.refresh_token
    global json_result
    next_url = config.next_url

    method = config.search_type or "search_illust"
    if next_url is not None:
        next_qs = api.parse_qs(next_url)
        if method == "search_illust" and next_qs.get("word") is None:
            next_qs["word"] = config.tag
        json_result = await object.__getattribute__(api, method)(**next_qs)
    else:
        if method == "illust_recommended":
            json_result = await api.illust_recommended(
                content_type="illust",
                offset=config.offset or 0,
                bookmark_illust_ids=config.recommend_base_illusts,
            )
        elif method == "search_illust" and config.tag is not None:
            json_result = await api.search_illust(
                config.tag,
                search_target="exact_match_for_tags",
                sort="popular_desc",
            )
        else:
            json_result = await api.illust_recommended(content_type="illust")
            config.search_type = "illust_recommended"

    if json_result is not None and json_result.get("next_url", None) and config.search_type == "search_illust":
        config.next_url = json_result.get("next_url")

    illusts = json_result.get("illusts", [])
    # config.offset = (config.offset or 0) + len(illusts)

    if len(illusts) == 0:
        print("ERROR: current searching method receive nothing")
        if config.next_url is not None:
            print("      ==> trying to remove next_url")
            config.next_url = None
            return await pixiv()
        if config.tag is not None:
            print("      ==> trying to remove tags")
            config.tag = None
            return await pixiv()
        if config.search_type == "search_illust":
            print("      ==> trying to change search method to illust_recommended")
            config.search_type = "illust_recommended"
            return await pixiv()
        if config.offset != 0:
            print("   ==> trying to change offset to 0")
            config.offset = 0
            return await pixiv()

        print("    ==> trying to relogin")
        api.refresh_token = None

    for i in illusts:
        # filter r18 images (based on tags)
        tags = i.get("tags", [])
        if should_skip(map(lambda x: x.get("name", None), tags), i):
            continue

        if config.maxPage is not None and i.get("page_count", 1) > config.maxPage:
            # skip if page count is too large, the image is probably a comic
            continue
        if i.get("page_count", 1) > 1:
            for j in i["meta_pages"]:
                queue.put_nowait(
                    PriorityEntry(
                        randint(0, 100),
                        _to_image(
                            j["image_urls"][
                                config.size if config.size else "original"
                            ],
                            i,
                        ),
                    )
                )
        else:
            if not config.size or config.size == "original":
                url = i["meta_single_page"].get("original_image_url", None)
            else:
                url = i["image_urls"].get(config.size, None)
            queue.put_nowait(PriorityEntry(randint(0, 100), _to_image(url, i)))
    return await pixiv()


@router.get("/pixiv/like")
async def pixiv_like(id: int):
    await api.illust_bookmark_add(id)

    return {"status": "ok"}


@router.get("/pixiv/proxy/{rest:path}")
def proxy_pixiv(rest: str):
    url = "https://i.pximg.net/" + rest

    for i in range(3):
        response = requests.get(
            url,
            stream=True,
            proxies={"http": config.proxy, "https": config.proxy} if config.proxy else None,
            headers={"Referer": "https://www.pixiv.net/"},
        )
        if response.status_code == 200:
            break
        else:
            print(f"ERROR: {response.status_code} when fetching {url}")
            print(f"       retrying... ({i + 1}/3)")
    else:
        raise Exception("Failed to fetch image")

    def get_image():
        for chunk in response.iter_content(chunk_size=1024):
            yield chunk

    return StreamingResponse(
        get_image(),
        media_type=response.headers["Content-Type"],
    )
