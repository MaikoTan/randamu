from fastapi import APIRouter

from . import lolicon
from . import pixiv
from . import stable_horde

router = APIRouter(prefix="/image")

router.include_router(lolicon.router)
router.include_router(pixiv.router)
router.include_router(stable_horde.router)
