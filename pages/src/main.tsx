import { createElement as h, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";

function Randamu() {
  const [service, setService] = useState("pixiv");
  const [interval, setInterval] = useState(30);
  type Bg = Record<'title' | 'url' | 'page_url' | 'author' | 'author_url' | 'data_url', string>
  const [bg, setBg] = useState<Bg>();
  const [nextBg, setNextBg] = useState<Bg>()

  useEffect(() => {
    const fetchConfig = async () => {
      var resp = await fetch("/api/config", { mode: "no-cors" });
      var json = await resp.json();

      setService(json.service ?? "pixiv");
      setInterval(json.interval ?? 30);
    };

    fetchConfig().catch(console.error);
  }, []);

  async function getBg() {
    const res = await fetch(`/api/image/${service}?image=1`, { mode: "no-cors" });
    const r = await res.json();
    return r;
  }

  function preload() {
    getBg().then((r) => {
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
        setBg(r);
        preload();
      });
    }
  }, [service]);

  useEffect(() => {
    const id = window.setTimeout(() => {
      setBg(nextBg)
      preload()
    }, interval * 1000)

    return () => clearTimeout(id)
  }, [interval, nextBg])

  // FIXME: `next()` should clear timer, otherwise timer would still
  // count the original secondes.
  async function next() {
    const nb = await getBg()
    setBg(nextBg)
    setNextBg(nb)
    if (!nb.data_url)
      new Image().src = nb.url
  }

  if (!bg) {
    return <div>Requesting ...</div>;
  }
  return (<div>
    <div id="bg" style={{ backgroundImage: `url(${bg.data_url ?? bg.url})`}} />
    <a id="fg" style={{ backgroundImage: `url(${bg.data_url ?? bg.url})` }} onClick={next} />
    <div id="info">
      <a id="title" href={ bg.page_url ?? "#" } target="_blank">{ bg.title ?? "" }</a>
      <a
        id="author"
        href={ bg.author_url ?? "#" }
        target="_blank"
      >
        { bg.author ?? "" }
      </a>
    </div>
  </div>)
}

const root = createRoot(document.querySelector("#app"));

root.render(h(Randamu));
