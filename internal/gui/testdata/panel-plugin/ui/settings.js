// A panel on Settings' Plugins tab: how many notes there are, and a way to
// clear them.
(async () => {
  await agentlink.ready;
  let opts = await agentlink.options.get();
  const paint = () => {
    const n = Object.values(opts.notes || {}).filter(Boolean).length;
    document.getElementById("count").textContent = n === 1 ? "1 note" : `${n} notes`;
  };
  paint();
  document.getElementById("clear").onclick = async () => {
    opts = await agentlink.options.set({ ...opts, notes: {} });
    paint();
  };
})();
