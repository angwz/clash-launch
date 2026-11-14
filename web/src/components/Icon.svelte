<script lang="ts" module>
  // 本项目自绘的线性图标（24 栅格、1.75 描边），不引入图标库。
  // 有方向含义的图标（chevron-right、arrow-right、logout、external）在 RTL 下水平镜像（第 15.3 节）。
  const paths = {
    logo: "M12 2.5c3 2.2 4.6 5.6 4.6 9.3v3.7H7.4v-3.7c0-3.7 1.6-7.1 4.6-9.3ZM12 9.2a1.6 1.6 0 1 0 0 3.2 1.6 1.6 0 0 0 0-3.2ZM7.4 13.2 4.5 16v3l2.9-1.4M16.6 13.2l2.9 2.8v3l-2.9-1.4M10 18.5c.4 1.4 1.1 2.4 2 3 .9-.6 1.6-1.6 2-3",
    menu: "M4 6.5h16M4 12h16M4 17.5h16",
    search: "M10.5 4a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM15.3 15.3 20 20",
    plus: "M12 5v14M5 12h14",
    minus: "M5 12h14",
    x: "M6 6l12 12M18 6 6 18",
    check: "M5 12.5l4.5 4.5L19 7.5",
    "chevron-down": "M6 9.5l6 6 6-6",
    "chevron-up": "M6 14.5l6-6 6 6",
    "chevron-right": "M9.5 6l6 6-6 6",
    "arrow-up": "M12 19V5M6 11l6-6 6 6",
    "arrow-down": "M12 5v14M6 13l6 6 6-6",
    "arrow-right": "M5 12h14M13 6l6 6-6 6",
    error: "M12 3.5a8.5 8.5 0 1 0 0 17 8.5 8.5 0 0 0 0-17ZM12 7.5v5.5M12 16.2v.3",
    warning: "M12 3.8 2.8 19.5h18.4L12 3.8ZM12 9.5v4.5M12 16.8v.3",
    info: "M12 3.5a8.5 8.5 0 1 0 0 17 8.5 8.5 0 0 0 0-17ZM12 11v5.5M12 7.8v.3",
    ok: "M12 3.5a8.5 8.5 0 1 0 0 17 8.5 8.5 0 0 0 0-17ZM8 12.3l2.8 2.8L16.2 9.5",
    bolt: "M13.2 2.8 5 13.5h6l-1.2 7.7L18 10.5h-6l1.2-7.7Z",
    sun: "M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM12 2.5v2.2M12 19.3v2.2M2.5 12h2.2M19.3 12h2.2M5.3 5.3l1.5 1.5M17.2 17.2l1.5 1.5M5.3 18.7l1.5-1.5M17.2 6.8l1.5-1.5",
    moon: "M19.5 14.2A7.8 7.8 0 0 1 9.8 4.5a7.8 7.8 0 1 0 9.7 9.7Z",
    monitor: "M3.5 5h17v11h-17zM8.5 20h7M12 16v4",
    globe: "M12 3.5a8.5 8.5 0 1 0 0 17 8.5 8.5 0 0 0 0-17ZM3.5 12h17M12 3.5c2.3 2.4 3.4 5.2 3.4 8.5s-1.1 6.1-3.4 8.5c-2.3-2.4-3.4-5.2-3.4-8.5s1.1-6.1 3.4-8.5Z",
    user: "M12 4a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM4.5 20.5c.8-3.6 3.8-6 7.5-6s6.7 2.4 7.5 6",
    logout: "M14 4.5h4.5v15H14M10 8l-4 4 4 4M6 12h9",
    copy: "M8.5 8.5h11v11h-11zM15.5 8.5v-4h-11v11h4",
    link: "M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3A4 4 0 0 0 11 18.7l1-1",
    refresh: "M19.5 12a7.5 7.5 0 1 1-2.2-5.3M19.5 4.5v4h-4",
    trash: "M4.5 7h15M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13M10.2 11v5.5M13.8 11v5.5",
    grip: "M9 6.5v.1M15 6.5v.1M9 12v.1M15 12v.1M9 17.5v.1M15 17.5v.1",
    external: "M13.5 4.5h6v6M19.5 4.5l-8 8M17 14v5.5H4.5V7H10",
    file: "M6 3.5h8l4 4v13H6zM14 3.5v4h4",
    layers: "M12 3.5 3 8.2l9 4.7 9-4.7-9-4.7ZM3 12.2l9 4.7 9-4.7M3 16.2l9 4.7 9-4.7",
    server: "M4 4.5h16v6H4zM4 13.5h16v6H4zM7.5 7.5h.1M7.5 16.5h.1",
    rules: "M5 6.5h.1M9 6.5h10M5 12h.1M9 12h10M5 17.5h.1M9 17.5h10",
    shield: "M12 3 4.5 6v5.5c0 4.6 3.1 8.2 7.5 9.5 4.4-1.3 7.5-4.9 7.5-9.5V6L12 3Z",
    history: "M4.5 12a7.5 7.5 0 1 0 2.2-5.3M4.5 4.5v4h4M12 8v4.5l3 2",
    qr: "M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2.5v2.5H14zM17.5 17.5H20V20h-2.5zM14 18.5v1.5h1.5M18.5 14H20v1.5",
    eye: "M2.5 12s3.5-6.5 9.5-6.5 9.5 6.5 9.5 6.5-3.5 6.5-9.5 6.5S2.5 12 2.5 12ZM12 9.2a2.8 2.8 0 1 0 0 5.6 2.8 2.8 0 0 0 0-5.6Z",
    "eye-off": "M4 4l16 16M10 5.7c.6-.1 1.3-.2 2-.2 6 0 9.5 6.5 9.5 6.5a17 17 0 0 1-2.9 3.6M14.1 14.1A2.8 2.8 0 0 1 9.9 9.9M6.6 7.2C4 8.9 2.5 12 2.5 12s3.5 6.5 9.5 6.5c1.6 0 3-.5 4.3-1.1",
    more: "M6 12h.1M12 12h.1M18 12h.1",
    settings: "M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6ZM19 12l1.8-1-1.5-3-2 .3-1.4-1.4.3-2-3-1.5-1 1.8h-1.6l-1-1.8-3 1.5.3 2-1.4 1.4-2-.3-1.5 3L5 12l-1.8 1 1.5 3 2-.3 1.4 1.4-.3 2 3 1.5 1-1.8h1.6l1 1.8 3-1.5-.3-2 1.4-1.4 2 .3 1.5-3L19 12Z",
    command: "M8 4.5a2.5 2.5 0 1 0 0 5h2.5V7A2.5 2.5 0 0 0 8 4.5ZM16 4.5A2.5 2.5 0 0 0 13.5 7v2.5H16a2.5 2.5 0 1 0 0-5ZM8 19.5a2.5 2.5 0 1 1 0-5h2.5V17A2.5 2.5 0 0 1 8 19.5ZM16 19.5a2.5 2.5 0 0 1-2.5-2.5v-2.5H16a2.5 2.5 0 1 1 0 5ZM10.5 9.5h3v5h-3z",
    panel: "M3.5 4.5h17v15h-17zM14.5 4.5v15",
    upload: "M12 15.5V4.5M7 9l5-5 5 5M4.5 15v4.5h15V15",
    download: "M12 4.5v11M7 11l5 5 5-5M4.5 15v4.5h15V15",
    code: "M8.5 7 3.5 12l5 5M15.5 7l5 5-5 5",
    flag: "M5.5 21V4M5.5 4.5h11l-2.2 4 2.2 4h-11",
    // github 取自 Feather Icons（MIT），与本集合同为 24 栅格线性风格
    github: "M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22",
    sparkle: "M12 3.5c.6 4.2 2.3 5.9 6.5 6.5-4.2.6-5.9 2.3-6.5 6.5-.6-4.2-2.3-5.9-6.5-6.5 4.2-.6 5.9-2.3 6.5-6.5ZM18.5 15.5c.3 1.6 1 2.3 2.5 2.5-1.6.3-2.3 1-2.5 2.5-.3-1.6-1-2.3-2.5-2.5 1.6-.3 2.3-1 2.5-2.5Z",
  } as const;
  export type IconName = keyof typeof paths;
  const mirrored = new Set<IconName>(["chevron-right", "arrow-right", "logout", "external"]);
</script>

<script lang="ts">
  let { name, size = 16, label }: { name: IconName; size?: number; label?: string } = $props();
</script>

<svg
  class="icon"
  class:mirror={mirrored.has(name)}
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.75"
  stroke-linecap="round"
  stroke-linejoin="round"
  role={label ? "img" : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : "true"}
  focusable="false"><path d={paths[name]} /></svg>

<style>
  .icon {
    flex: none;
    display: inline-block;
    vertical-align: middle;
  }
  :global([dir="rtl"]) .mirror {
    transform: scaleX(-1);
  }
</style>
