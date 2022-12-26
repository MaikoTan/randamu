from typing import Optional
import requests
from fastapi import FastAPI
from starlette.responses import FileResponse 

app = FastAPI()

@app.get("/api")
def main(tag=None, r18=0, num=1, excludeAI=True, dateAfter: Optional[int]=None, dateBefore: Optional[int]=None):
    payload = {
        "r18": r18,
        "num": num,
        "excludeAI": excludeAI,
    }
    if tag is not None:
        payload["tag"] = [tag]
    if dateAfter is not None:
        payload["dateAfter"] = dateAfter
    if dateBefore is not None:
        payload["dateBefore"] = dateBefore
    r = requests.post("https://api.lolicon.app/setu/v2", json=payload)
    j = r.json()
    i = j["data"][0]
    url: str = i["urls"]["original"]
    return { "url": url }

@app.get("/")
async def read_index():
    return FileResponse('index.html')

@app.get("/user.css")
async def read_user_css():
    if os.path.exists("user.css"):
        return FileResponse("user.css")
    else:
        return ""
