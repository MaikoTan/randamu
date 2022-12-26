import os

from fastapi import APIRouter
from starlette.responses import FileResponse

from . import api

router = APIRouter()

router.include_router(api.router)

@router.get("/")
async def read_index():
    return FileResponse('index.html')

@router.get("/user.css")
async def read_user_css():
    if os.path.exists("user.css"):
        return FileResponse("user.css")
    else:
        return ""

@router.get("/config")
async def read_config():
    return FileResponse("config.html")
