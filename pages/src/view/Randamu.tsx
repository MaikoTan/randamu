import styled from "@emotion/styled";
import { css } from "@emotion/react";
import { useEffect, useState } from "react";

declare module "csstype" {
  interface Properties {
    "--background-image"?: string;
  }
}

type Bg = Record<"title" | "url" | "page_url" | "author" | "author_url", string>;

const Requesting = styled.div`
  width: 100vw;
  height: 100vh;
  display: flex;
  justify-content: space-between;
  align-items: center;
`;

const Bg = styled.div`
  width: 100vw;
  height: 100vh;
  background-image: var(--background-image);
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
  filter: blur(10px);
`;

const Fg = styled(Bg.withComponent("a"))`
  position: absolute;
  top: 0;
  left: 0;
  background-size: contain;
  transition: background-image 0.5s ease-in-out;
  text-decoration: none;
  display: block;
  filter: unset;
`;

const Title = (props: any) => {
  return (
    <a
      target="_blank"
      css={css`
        color: #fff;
        display: block;
        text-decoration: none;
      `}
      {...props}
    />
  );
};

const Author = styled(Title)`
  font-size: 0.7em;
`;

const Info = (props: { image: Bg }) => {
  const { image } = props;
  return (
    <div
      css={css`
        position: absolute;
        top: 0;
        left: 0;
        width: 100%;
        padding: 5px 10px;
        line-height: 1.3em;
        background: rgba(0, 0, 0, 0.5);
        transition: background 0.5s ease-in-out;
      `}
    >
      <Title href={image.page_url ?? "#"}>{image.title ?? ""}</Title>
      <Author href={image.author_url ?? "#"}>{image.author ?? ""}</Author>
    </div>
  );
};

export default function Randamu() {
  const [service, setService] = useState("pixiv");
  const [interval, setInterval] = useState(30);
  const [bg, setBg] = useState<Bg>();
  const [nextBg, setNextBg] = useState<Bg>();

  useEffect(() => {
    const fetchConfig = async () => {
      var resp = await fetch("/api/config", { mode: "no-cors" });
      var json = await resp.json();

      setService(json.service ?? "pixiv");
      setInterval(json.interval ?? 30);
    };

    fetchConfig().catch(console.error);
  }, []);

  const getBg = async () => {
    const res = await fetch(`/api/image/${service}`, { mode: "no-cors" });
    const r = await res.json();
    return r;
  };

  const preload = () => {
    getBg().then((r) => {
      setNextBg(r);
      // preload image
      new Image().src = r.url;
    });
  };

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
      setBg(nextBg);
      preload();
    }, interval * 1000);

    return () => clearTimeout(id);
  }, [interval, nextBg]);

  if (!bg) {
    return <Requesting>Requesting ...</Requesting>;
  }
  return (
    <div style={{ "--background-image": `url(${bg.url})` }}>
      <Bg />
      <Fg href="#" />
      <Info image={bg} />
    </div>
  );
}
