import requests
from fastapi import FastAPI
from starlette.responses import FileResponse 

app = FastAPI()

@app.get("/api")
def main(tag=None, r18=0, num=1, excludeAI=True):
    payload = {
        "r18": r18,
        "num": num,
        "excludeAI": excludeAI,
    }
    if tag is not None:
        payload["tag"] = [tag]
    r = requests.post("https://api.lolicon.app/setu/v2", json=payload)
    j = r.json()
    i = j["data"][0]
    url: str = i["urls"]["original"]
    return { "url": url }

@app.get("/")
async def read_index():
    return FileResponse('index.html')