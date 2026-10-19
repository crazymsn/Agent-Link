// What agentlink's window gives a page: the theme, its own options, and the
// reads package.json's agentlink.permissions declares.
(async () => {
  await agentlink.ready;
  const opts = await agentlink.options.get();
  const s = await agentlink.read("usage:summary", { period: "today" });
  document.getElementById("calls").textContent = `${s.calls} calls, ${s.input + s.output} tokens`;
  const cost = document.getElementById("cost");
  const paint = (hide) => { cost.textContent = hide ? "" : `$${s.cost.toFixed(2)}`; };
  paint(opts.hideCost);
  document.getElementById("hide").onclick = async () => {
    const next = await agentlink.options.set({ ...opts, hideCost: !opts.hideCost });
    Object.assign(opts, next);
    paint(opts.hideCost);
  };
})();
