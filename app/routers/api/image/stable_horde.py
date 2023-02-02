import asyncio
from typing import Any, Optional
import aiohttp

from fastapi import APIRouter

from app.models.image import Image
from app.config import Config

config = Config("stable-horde")

router = APIRouter()


class StableHordeAPI:
    def __init__(self, token: str):
        self.token = token
        self.session: Optional[aiohttp.ClientSession] = None

    async def _request(self, path: str, method: str = "GET", payload: Any = None):
        if self.session is None:
            self.session = aiohttp.ClientSession(headers={"apikey": self.token})
        async with self.session.request(
            method, f"https://stablehorde.net/api/v2{path}", json=payload
        ) as resp:
            return await resp.json()

    async def txt2img_request(self, payload: Any):
        resp = await self._request("/generate/async", "POST", payload)
        id = resp.get("id")
        if id is not None:
            return id
        else:
            raise Exception(f"Error: {resp.get('message', 'Unknown error')}")

    async def generate_check(self, id: str):
        resp = await self._request(f"/generate/check/{id}")
        return resp.get("done") == True

    async def generate_status(self, id: str):
        resp = await self._request(f"/generate/status/{id}")
        return resp.get("generations", [])


client = StableHordeAPI(config.token)


@router.get("/stable-horde", response_model=Image)
async def horde():
    payload = {
        "prompt": f"{config.prompt} ### {config.negative_prompt}",
        "params": {
            "sampler_name": config.sampler or "k_euler_a",
            "cfg_scale": 7,
            "height": config.height,
            "width": config.width,
            "steps": config.steps or 30,
            "karras": True,
            "post_processing": ["RealESRGAN_x4plus"] if config.upscale else [],
        },
        "nsfw": bool(config.nsfw),
        "censor_nsfw": bool(config.censor_nsfw),
        "workers": config.workers or [],
        "models": config.models or ["Anything Diffusion"],
        "n": 1,
        "r2": True,
    }
    print(payload)
    # payload can also be a dict, which is useful, if something new added
    uuid = await client.txt2img_request(payload)

    while not await client.generate_check(uuid):
        # Checking every second if image is generated
        await asyncio.sleep(1)

    # Generating a status which has all generations
    generations = await client.generate_status(uuid)

    image = generations[0].get("img")

    return Image(
        url=image,
        title="StableHorde",
        author="*",
        page_url="https://stablehorde.net/",
        author_url="https://stablehorde.net/",
        data={},
    )
