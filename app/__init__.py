from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles

from app import routers

app = FastAPI()

app.include_router(routers.router)
app.mount('/', StaticFiles(directory='pages'), name='static')
