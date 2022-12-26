from asyncio import Queue
import json
from fastapi import APIRouter

from pixivpy_async import AppPixivAPI

router = APIRouter()

api = AppPixivAPI(proxy="socks5://127.0.0.1:7891")

with open("config.json", "r") as f:
    config = json.load(f)

queue: Queue[str] = Queue()

@router.get("/pixiv")
async def pixiv():
    if queue.empty():
        if api.refresh_token is None:
            await api.login_web()
        illusts = await api.illust_recommended(content_type="illust")
        for i in illusts["illusts"]:
            url = i["meta_single_page"].get("original_image_url", None)
            if url is None:
                for j in i["meta_pages"]:
                    queue.put_nowait(j["image_urls"]["original"])
            else:
                queue.put_nowait(url)
    return { "url": queue.get_nowait().replace("i.pximg.net", "i.pixiv.re") }
