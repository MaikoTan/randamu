import json
import os
from fastapi import APIRouter, Form

from app.models import RandamuConfig

router = APIRouter()

@router.get("/api/config")
def config():
    if not os.path.exists("config.json"):
        with open("config.json", "w") as f:
            f.write("{}")
    with open("config.json", "r") as f:
        return json.load(f)

@router.post("/api/config")
def set_config(data: RandamuConfig = Form()):
    with open("config.json", "w") as f:
        json.dump(data, f)
    return { "code": 0 }
