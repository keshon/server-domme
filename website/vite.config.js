import { defineConfig } from 'vite';

export default defineConfig({
  base: process.env.SITE_BASE_PATH || '/',
});
