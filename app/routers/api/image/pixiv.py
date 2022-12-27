from asyncio import Queue
from random import shuffle
import json
from typing import Any, Dict, Optional
from fastapi import APIRouter

from pixivpy_async import AppPixivAPI

router = APIRouter()

api = AppPixivAPI(proxy="socks5://127.0.0.1:7891")

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

class ShuffleQueue(Queue):
    def get_nowait(self):
        shuffle(self._queue)
        return super().get_nowait()

queue: Queue[Any] = ShuffleQueue()

json_result: Optional[Dict[str, Any]] = None

@router.get("/pixiv")
async def pixiv():
    if queue.empty():
        if config.refresh_token is not None:
            await api.login(refresh_token=config.refresh_token)
        if api.refresh_token is None:
            await api.login_web()
        recommend_base_illusts = config.recommend_base_illusts
        global json_result
        next_qs = None
        if json_result is not None and json_result.next_url:
            next_qs = api.parse_qs(json_result.next_url)
        if next_qs is not None:
            json_result = await api.illust_recommended(**next_qs)
        if recommend_base_illusts is None:
            json_result = await api.illust_recommended(content_type="illust")
        else:
            json_result = await api.illust_recommended(content_type="illust", bookmark_illust_ids=recommend_base_illusts)
        for i in json_result["illusts"]:
            # filter r18 images (based on tags)
            tags = i.get("tags", [])
            skip = False
            blacklist = config.blacklist if config.blacklist else []
            for t in tags:
                if config.r18 == False and t.get("name", "") == "R-18":
                    skip = True
                    break
                if len(blacklist) and t.get("name", "") in blacklist:
                    skip = True
                    break
            if skip:
                continue

            url = i["meta_single_page"].get("original_image_url", None)
            if url is None:
                for j in i["meta_pages"]:
                    queue.put_nowait({
                        "url": j["image_urls"]["original"].replace("i.pximg.net", "i.pixiv.re"),
                        "data": i,
                    })
            else:
                queue.put_nowait({ "url": url.replace("i.pximg.net", "i.pixiv.re"), "data": i })
    return queue.get_nowait()
