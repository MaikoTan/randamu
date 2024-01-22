import { createElement as h, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";

// get api root from current url query param `api_root`
const API_ROOT = new URLSearchParams(window.location.search).get("api_root") ?? "http://localhost:8089/api";

function Randamu() {
  const [service, setService] = useState();
  const [interval, setInterval] = useState(30);
  const [metainfo, setMetainfo] = useState<{ enable: boolean; position: 'top' | 'bottom' }>({ enable: true, position: 'top' })
  type Bg = Record<'title' | 'url' | 'page_url' | 'author' | 'author_url' | 'data_url', string> & { pixiv_id?: number }
  const [bg, setBg] = useState<Bg>();
  const [nextBg, setNextBg] = useState<Bg>()
  const [like, setLike] = useState(false)

  useEffect(() => {
    const fetchConfig = async () => {
      var resp = await fetch("/api/config", { mode: "no-cors" });
      var json = await resp.json();

      setService(json.service);
      setInterval(json.interval);
      if (json.metainfo?.enable === false) {
        setMetainfo({ ...metainfo, enable: false })
      }
      if (json.metainfo?.position) {
        setMetainfo({ ...metainfo, position: json.metainfo.position })
      }
    };

    fetchConfig().catch(console.error);
  }, []);

  async function getBg() {
    if (!service) return;
    const res = await fetch(API_ROOT + `/image/${service}`, { mode: "no-cors" });
    const r = await res.json();
    return r;
  }

  function preload() {
    getBg().then((r) => {
      if (!r) return;
      setNextBg(r)
      // preload image
      if (!r.data_url)
        new Image().src = r.url;
    });
  }

  useEffect(() => {
    if (!bg) {
      // load the first one
      getBg().then((r) => {
        if (!r) return;
        setLike(false)
        setBg(r);
        preload();
      });
    }
  }, [service]);

  useEffect(() => {
    const id = window.setTimeout(() => {
      setLike(false)
      setBg(nextBg)
      preload()
    }, interval * 1000)

    return () => clearTimeout(id)
  }, [interval, nextBg])

  // FIXME: `next()` should clear timer, otherwise timer would still
  // count the original secondes.
  async function next() {
    const nb = await getBg()
    setLike(false)
    setBg(nextBg)
    setNextBg(nb)
    if (!nb.data_url)
      new Image().src = nb.url
  }

  async function doLike() {
    if (!bg?.pixiv_id) return;
    await fetch(API_ROOT + `/image/pixiv/like?id=${bg.pixiv_id}`, { mode: "no-cors" });
    setLike(true)
  }

  useEffect(() => {
    const listener = (e: KeyboardEvent) => {
      if (e?.key === "l" || e?.key === "L") {
        e.preventDefault()
        doLike()
      } else if (e?.key === "n" || e?.key === "N") {
        e.preventDefault()
        next()
      }
    }
    document.addEventListener("keydown", listener)

    return () => document.removeEventListener("keydown", listener)
  })

  if (!bg?.url) {
    return <div>Requesting ...</div>;
  }

  const bgImage = `url(${bg.data_url ?? bg.url})`
  return (<div>
    <div id="bg" style={{ backgroundImage: bgImage }} />
    <div id="fg" style={{ backgroundImage: bgImage }} />
    <div id="info" className={[!metainfo.enable ? 'hide': '', metainfo.position === 'bottom' ? 'bottom' : ''].join(' ')}>
      <a id="title" href={ bg.page_url ?? "#" } target="_blank">{ bg.title ?? "" }</a>
      <br />
      <a
        id="author"
        href={ bg.author_url ?? "#" }
        target="_blank"
      >
        { bg.author ?? "" }
      </a>
      <a id="next" href="#" onClick={next}>➡️</a>
      <a id="like" href="#" onClick={doLike}>
        { like ? '❤️' : '💙' }
      </a>
    </div>
  </div>)
}

const root = createRoot(document.querySelector("#app"));

root.render(h(Randamu));
