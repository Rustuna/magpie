// Run with Node's test runner and Playwright on the module path; see README.md.
// #5670: a project in a repository whose .gitignore is tracked had magpie's
// lines written into it, leaving every checkout dirty. The project card's
// "Ignore lines go to" control offers the .gitignore, the repository's
// exclude file, or nowhere: picking one posts projects/ignore with it, the
// footer says where the lines are, and the exclude option is disabled for a
// project in no git repository. Pressing an option scrolls nothing; in
// English and Chinese. No backend: the API is faked here.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const HOME = "/Users/emo";
const APP = HOME + "/code/app", NOTES = HOME + "/notes";
const agent = (id, name, more = {}) => ({ id, name, icon: "", skills: `${HOME}/.${id}/skills`, mcp: `${HOME}/.${id}/mcp.json`, ...more });
const lib = (projects) => ({
  dir: `${HOME}/.magpie/library`, backups: `${HOME}/.magpie/backups`, home: HOME,
  agents: [
    agent("claude", "Claude Code", { projectSkills: ".claude/skills", projectMCP: ".mcp.json" }),
    agent("codex", "Codex", { projectSkills: ".agents/skills", projectMCP: ".codex/config.toml" }),
  ],
  instructions: { agents: [] }, servers: [],
  skills: [{ name: "pdf", description: "Read PDFs", kind: "folder", agents: ["claude", "codex"] }],
  foundServers: [], foundSkills: [],
  projects,
});
const projects = () => [
  { dir: APP, name: "app", copy: false, ignore: "gitignore", git: true, skills: { pdf: ["claude", "codex"] }, placed: [".agents/skills/pdf", ".claude/skills/pdf"], servers: {}, wrote: {} },
  { dir: NOTES, name: "notes", copy: false, ignore: "gitignore", git: false, skills: {}, placed: [], servers: {}, wrote: {} },
];

function server(lang, posts) {
  let ps = projects();
  return async (route) => {
    const req = route.request(), url = new URL(req.url());
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return route.fulfill({ json: { agents: [], profiles: [], settings: { lang, theme: "light" } } });
    // the first read is a glance, drawn at once, the whole page after it
    if (url.pathname === "/api/library") return route.fulfill({ json: { ...lib(ps), partial: url.searchParams.get("glance") === "1" } });
    if (url.pathname === "/api/library/projects/ignore") {
      const b = req.postDataJSON();
      posts.push(b);
      ps = ps.map((p) => (p.dir === b.dir ? { ...p, ignore: b.ignore } : p));
      return route.fulfill({ json: { ...lib(ps), result: { changed: [], problems: [] } } });
    }
    if (url.pathname === "/api/plugins") return route.fulfill({ json: { plugins: [] } });
    if (url.pathname === "/api/usage/quotas") return route.fulfill({ json: [] });
    if (url.pathname === "/api/groups") return route.fulfill({ json: { groups: [] } });
    if (url.pathname === "/api/providers") return route.fulfill({ json: { providers: [], gateway: { running: true } } });
    if (url.pathname.startsWith("/api/")) return route.fulfill({ json: {} });
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const scrolled = (page) => page.evaluate(() => [window.scrollX, window.scrollY, ...[...document.querySelectorAll("*")].filter((e) => e.scrollTop || e.scrollLeft).map((e) => `${e.className}:${e.scrollTop},${e.scrollLeft}`)].join(" "));
// a click where the element is, as the reader's: Playwright's own click
// first scrolls it into view
const press = async (page, loc) => {
  const b = await loc.boundingBox();
  await page.mouse.click(b.x + b.width / 2, b.y + b.height / 2);
};

const words = {
  en: {
    label: "Ignore lines go to", gitignore: "Its .gitignore", exclude: "The repository's exclude file", none: "Nowhere",
    inGitignore: "listed in its .gitignore", inExclude: "listed in the repository's .git/info/exclude", inNone: "listed in no ignore file",
    toExclude: "app's ignore lines go to the repository's exclude file now", toNone: "app gets no ignore lines now",
    noRepo: "Not in a git repository",
  },
  zh: {
    label: "忽略规则写入", gitignore: "它的 .gitignore", exclude: "仓库的 exclude 文件", none: "不写入",
    inGitignore: "写入其 .gitignore", inExclude: "写入仓库的 .git/info/exclude", inNone: "不写入任何忽略文件",
    toExclude: "app 的忽略规则现在写入仓库的 exclude 文件", toNone: "app 现在不写入忽略规则",
    noRepo: "不在 git 仓库中",
  },
};

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  test(engine + ": where a project's ignore lines go", async (t) => {
    assert(["chromium", "webkit"].includes(engine), "BROWSER must be chromium or webkit");
    const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
    const errors = [];
    t.after(async () => {
      if (errors.length) console.log(errors);
      await browser.close();
    });
    for (const lang of ["en", "zh"]) {
      await t.test(lang, async () => {
        const w = words[lang], posts = [];
        const ctx = await browser.newContext({ viewport: { width: 860, height: 700 } });
        await ctx.addInitScript(() => { try { localStorage.setItem("magpie.libTab", "skills"); } catch {} });
        const page = await ctx.newPage();
        page.setDefaultTimeout(5000);
        page.setDefaultNavigationTimeout(60000);
        page.on("pageerror", (e) => errors.push(e.message));
        await page.route("**/*", server(lang, posts));
        await page.goto("http://magpie.test/");
        await page.locator('button[data-view="library"]').click();
        const cards = page.locator("#view-library .lib-project");
        await cards.nth(1).waitFor();

        // the project in a repository: opened, its control says the .gitignore
        const app = cards.filter({ hasText: "app" }).first();
        await press(page, app.locator(".lib-projhead"));
        const ctl = app.locator(".lib-projignore");
        await ctl.waitFor();
        assert.equal((await ctl.locator(".lib-projcopy").textContent()).trim(), w.label);
        const opts = ctl.locator(".segs .opt");
        assert.deepEqual((await opts.allTextContents()).map((s) => s.trim()), [w.gitignore, w.exclude, w.none]);
        assert.equal(await opts.nth(0).getAttribute("aria-pressed"), "true");
        assert.equal(await opts.nth(1).isDisabled(), false);
        const foot = app.locator(".lib-projfoot").first();
        assert.match(await foot.textContent(), new RegExp(w.inGitignore));
        assert.equal(await ctl.evaluate((e) => getComputedStyle(e).borderLeftWidth), "0px", "no stripe");

        // the exclude file picked: posted, said, and the footer follows
        const before = await scrolled(page);
        await press(page, opts.nth(1));
        await page.waitForFunction((s) => document.querySelector("#status").textContent.trim() === s, w.toExclude);
        assert.deepEqual(posts, [{ dir: APP, ignore: "exclude" }]);
        assert.equal(await scrolled(page), before, "pressing an option scrolls nothing");
        const appAgain = cards.filter({ hasText: "app" }).first();
        await page.waitForFunction((s) => document.querySelector("#view-library .lib-project .lib-projfoot")?.textContent.includes(s), w.inExclude);
        assert.equal(await appAgain.locator(".lib-projignore .segs .opt").nth(1).getAttribute("aria-pressed"), "true");

        // nowhere: the same again
        await press(page, appAgain.locator(".lib-projignore .segs .opt").nth(2));
        await page.waitForFunction((s) => document.querySelector("#status").textContent.trim() === s, w.toNone);
        assert.deepEqual(posts, [{ dir: APP, ignore: "exclude" }, { dir: APP, ignore: "none" }]);
        await page.waitForFunction((s) => document.querySelector("#view-library .lib-project .lib-projfoot")?.textContent.includes(s), w.inNone);

        // a folder in no repository: the exclude file can't be picked, and says why
        await cards.filter({ hasText: "notes" }).first().locator(".lib-projhead .name").click();
        await page.locator("#view-library .lib-projhead.open").filter({ hasText: "notes" }).waitFor();
        const ex = cards.filter({ hasText: "notes" }).first().locator(".lib-projignore .segs .opt").nth(1);
        await ex.waitFor();
        assert.equal(await ex.isDisabled(), true);
        assert.match(await ex.getAttribute("title"), new RegExp(w.noRepo));
        assert.deepEqual(errors, []);
        await ctx.close();
      });
    }
  });
}
