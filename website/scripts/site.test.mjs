import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';


test('website has a custom domain and keeps application state outside its sources', () => {
  assert(fs.readFileSync(new URL('../public/CNAME', import.meta.url), 'utf8').trim().endsWith('.keshon.ru'));
  assert(!fs.existsSync(new URL('../.env', import.meta.url)));
});
test('landing page remains a static application-independent page', () => {
  const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');
  assert(html.includes('https://server-domme.keshon.ru/'));
  assert(html.includes('assets/about-banner.webp'));
});
