from fastapi import FastAPI
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles

from app import routers

app = FastAPI()

app.include_router(routers.router)
app.mount('/', StaticFiles(directory='pages/dist', html=True), name='static')
