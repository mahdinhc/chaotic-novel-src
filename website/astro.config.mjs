import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';

// TODO: replace with your real domain before deploying
const SITE_URL = 'https://example.com';

export default defineConfig({
  site: SITE_URL,
  integrations: [sitemap()],
});
