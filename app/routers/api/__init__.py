from fastapi import APIRouter

from . import config
from . import image

router = APIRouter(prefix="/api")

router.include_router(config.router)
router.include_router(image.router)
