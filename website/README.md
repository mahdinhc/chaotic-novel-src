# Official source code of Chaötic! Webnovel Site

Astro + Markdown + Cloudflare Pages + GitHub.

One Markdown file = one chapter. Add a new file in `src/content/chapters/`
and it becomes a new page automatically — no HTML editing required.

## 1. Install

```
cd website
npm install
npm run dev
```

Then open the local URL Astro prints (usually http://localhost:4321).

## 2. Write a chapter

Copy an existing file in `src/content/chapters/`, e.g.:

```md
---
title: "Hello World"
chapter: "1"
volume: 1
date: "2016-06-27"
description: "-"
---

# Chapter 1: Hello world

He just punched himself...
```

## 3. Before deploying: set your real domain

Edit `astro.config.mjs` and replace `https://example.com` with your actual
domain — this feeds the canonical URLs, Open Graph tags, and sitemap.
Also update the `Sitemap:` line in `public/robots.txt` to match.


## 4. Deploy on Cloudflare Pages

Connect the GitHub repo or just drag and drop the `dist` after build.
