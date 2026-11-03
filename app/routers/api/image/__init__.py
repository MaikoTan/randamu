from fastapi import APIRouter

from . import lolicon
from . import pixiv

router = APIRouter(prefix="/image")

router.include_router(lolicon.router)
router.include_router(pixiv.router)
