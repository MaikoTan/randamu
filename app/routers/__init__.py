from fastapi import APIRouter

from . import api

router = APIRouter()

router.include_router(api.router)
