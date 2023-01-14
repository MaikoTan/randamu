(function () {
  function Randamu() {
    const { createElement: h, useEffect, useState } = React;

    const [service, setService] = useState("pixiv");
    const [interval, setInterval] = useState(30);
    const [bg, setBg] = useState();

    useEffect(() => {
      const fetchConfig = async () => {
        var resp = await fetch("/api/config", { mode: "no-cors" });
        var json = await resp.json();

        setService(json.service ?? "pixiv");
        setInterval(json.interval ?? 30);
      };

      fetchConfig().catch(console.error);
    }, []);

    useEffect(() => {
      let currentTimer;

      async function getBg() {
        const res = await fetch(`/api/image/${service}`, { mode: "no-cors" });
        const r = await res.json();
        return r;
      }

      function preload() {
        getBg().then((r) => {
          // preload image
          new Image().src = r.url;
          currentTimer = setTimeout(() => {
            setBg(r);
            preload();
          }, interval * 1000);
        });
      }

      // load the first one
      getBg().then((r) => {
        setBg(r);
        preload();
      });

      return () => {
        clearTimeout(currentTimer);
        currentTimer = null;
      };
    }, [service, interval]);

    if (!bg) {
      return h("div", {}, "Requesting ...");
    }
    return h("div", {}, [
      h("div", { id: "bg", style: { backgroundImage: `url(${bg.url})` } }),
      h("a", { id: "fg", style: { backgroundImage: `url(${bg.url})` }, href: "#" }),
      h("div", { id: "info" }, [
        h("a", { id: "title", href: bg.page_url ?? "#", target: "_blank" }, bg.title ?? ""),
        h(
          "a",
          {
            id: "author",
            href: bg.author_url ?? "#",
            target: "_blank",
          },
          bg.author ?? "",
        ),
      ]),
    ]);
  }

  const root = ReactDOM.createRoot(document.querySelector("#app"));

  root.render(React.createElement(Randamu));
})();
