import { createElement as h } from "react";
import { createRoot } from "react-dom/client";

import { Randamu } from './view'

const root = createRoot(document.querySelector("#app"));

root.render(h(Randamu));
