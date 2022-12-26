from fastapi import APIRouter

from . import lolicon

router = APIRouter(prefix="/image")

router.include_router(lolicon.router)
