// The docs' contents, language menu and reading position, shared by every
// language. Each page's aside.toc lists its own sections; this adds the other
// pages around them, follows the reading position, and on a phone turns the
// list into a menu under a bar that names the section being read.
(() => {
  const lang = (document.documentElement.lang || "en").slice(0, 2);
  const L = {
    en: { docs: "Docs", contents: "Contents" },
    zh: { docs: "文档", contents: "目录" },
    ja: { docs: "ドキュメント", contents: "目次" },
  }[lang] || { docs: "Docs", contents: "Contents" };
  const clean = (p) => p.replace(/\.html$/, "").replace(/\/+$/, "") || "/";
  const el = (tag, cls, text) => {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text) e.textContent = text;
    return e;
  };

  // language menu
  const langBtn = document.querySelector("nav button.lang");
  const langs = document.getElementById("langs");
  if (langBtn && langs) {
    const langOpen = (on) => { langs.hidden = !on; langBtn.setAttribute("aria-expanded", String(on)); };
    langBtn.addEventListener("click", () => langOpen(langs.hidden));
    langs.querySelectorAll("a").forEach((a) => {
      if (a.hreflang === lang) a.setAttribute("aria-current", "true");
      a.addEventListener("click", () => { document.cookie = "lang=" + a.hreflang + "; path=/; max-age=31536000; samesite=lax"; });
    });
    document.addEventListener("click", (e) => { if (!langBtn.parentNode.contains(e.target)) langOpen(false); });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape" && !langs.hidden) { langOpen(false); langBtn.focus(); } });
  }

  // contents: every docs page, this one's sections under it
  const toc = document.querySelector("aside.toc");
  if (!toc) return;
  const title = toc.querySelector("p")?.textContent.trim() || document.title;
  const secs = [...toc.querySelectorAll('a[href^="#"]')];
  const here = clean(location.pathname);
  let pages = [...document.querySelectorAll('nav .links a[href^="/docs/"]')].map((a) => ({ href: a.getAttribute("href"), text: a.textContent.trim() }));
  if (!pages.some((p) => clean(p.href) === here)) pages = [{ href: here, text: title }];

  toc.id = "toc";
  toc.textContent = "";
  toc.setAttribute("aria-label", L.contents);
  toc.append(el("p", "toc-h", L.docs));
  const list = el("ol");
  for (const p of pages) {
    const li = el("li", "pg");
    const a = el("a", "", p.text);
    a.href = p.href;
    li.append(a);
    if (clean(p.href) === here) {
      li.classList.add("cur");
      a.setAttribute("aria-current", "page");
      const sub = el("div", "secs");
      sub.append(...secs);
      li.append(sub);
    }
    list.append(li);
  }
  toc.append(list);

  // the phone's bar: "Contents" and the section being read
  const bar = el("div", "toc-bar");
  bar.innerHTML = '<div class="wrap"><button type="button" aria-expanded="false" aria-controls="toc"><span class="k"></span><span class="t"></span><svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M4 6l4 4 4-4"/></svg></button></div>';
  const btn = bar.querySelector("button");
  const label = bar.querySelector(".t");
  bar.querySelector(".k").textContent = L.contents;
  label.textContent = title;
  const scrim = el("div", "toc-scrim");
  scrim.hidden = true;
  document.querySelector("body > nav").after(bar);
  document.body.append(scrim);

  const show = (on) => {
    toc.classList.toggle("open", on);
    scrim.hidden = !on;
    btn.setAttribute("aria-expanded", String(on));
    if (on) { const cur = toc.querySelector(".secs a.on"); if (cur) keepInView(cur, true); }
  };
  btn.addEventListener("click", () => show(!toc.classList.contains("open")));
  scrim.addEventListener("click", () => show(false));
  toc.addEventListener("click", (e) => { if (e.target.closest("a")) show(false); });
  document.addEventListener("keydown", (e) => { if (e.key === "Escape" && toc.classList.contains("open")) { show(false); btn.focus(); } });
  matchMedia("(min-width: 901px)").addEventListener("change", (m) => { if (m.matches) show(false); });

  // scroll the list itself, never the page, to keep the current section shown
  // (the list is sticky or fixed, so it is each link's offsetParent)
  function keepInView(a, center) {
    if (toc.scrollHeight <= toc.clientHeight) return;
    const top = a.offsetTop, bottom = top + a.offsetHeight;
    if (center) toc.scrollTop = top - toc.clientHeight / 2;
    else if (top < toc.scrollTop + 40) toc.scrollTop = top - 40;
    else if (bottom > toc.scrollTop + toc.clientHeight - 40) toc.scrollTop = bottom - toc.clientHeight + 40;
  }

  // reading position
  const heads = secs.map((a) => document.getElementById(decodeURIComponent(a.hash.slice(1))));
  let last = null;
  const spy = () => {
    const pad = parseFloat(getComputedStyle(document.documentElement).scrollPaddingTop) || 84;
    let i = -1;
    heads.forEach((h, j) => { if (h && h.getBoundingClientRect().top <= pad + 24) i = j; });
    if (innerHeight + scrollY >= document.documentElement.scrollHeight - 4) i = heads.length - 1;
    const on = secs[i] || null;
    if (on === last) return;
    last = on;
    secs.forEach((a) => a.classList.toggle("on", a === on));
    label.textContent = on ? on.textContent : title;
    if (on && getComputedStyle(bar).display === "none") keepInView(on, false);
  };
  addEventListener("scroll", spy, { passive: true });
  addEventListener("resize", spy);
  spy();
})();
