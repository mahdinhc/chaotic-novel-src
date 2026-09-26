import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';

const SITE_URL = 'https://chaotic-novel.pages.dev';

export default defineConfig({
  site: SITE_URL,
  integrations: [sitemap()],
});
