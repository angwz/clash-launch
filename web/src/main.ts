import "./styles/tokens.css";
import "./styles/accents.css";
import "./styles/base.css";
import { mount } from "svelte";
import App from "./App.svelte";
import { applyStoredAccent } from "./lib/accents";

applyStoredAccent(); // 强调色（R28）：在加载偏好前先按本机记下的值显示

mount(App, { target: document.getElementById("app")! });
