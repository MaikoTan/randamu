from fastapi import APIRouter

from . import image

router = APIRouter(prefix="/api")

router.include_router(image.router)
