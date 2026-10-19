// The sponsors' own links on useagentlink.ai's "Sponsored by" and in the
// READMEs' Sponsors row. Neither page links a sponsor directly: each goes
// to useagentlink.ai/go/<id>/<where>, which counts the click and sends the
// browser here (worker.js), so a sponsor's clicks can be told per place.
//
//   id     a-z, 0-9 and "-"; the same as the sponsor's partners.js id when
//          it has an in-app seat too (whose /go links are keys and site)
//   home   where the home page's card sends the browser
//   readme where the READMEs' logo sends it (README.md, README.zh-CN.md)
//
// A link stays here after its seat ends: a copy of the README or the page
// may still point at it.
export const SPONSORS = [
  { id: "yylx", home: "https://yylx.io/?ref=Agent Link" },
  { id: "apinebula", home: "https://apinebula.ai/ref/Gs54HwyB", readme: "https://apinebula.ai/ref/qmXqFGpr" },
  { id: "zerotoken", readme: "https://zzq.si" },
];
