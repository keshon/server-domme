# server-domme website

The website lives here; its Node dependencies are separate from the Go app.
Use Node.js 22.12+ or Node.js 24 LTS. Ruby and WSL are not required.

## Local development

From the repository root:

```sh
npm --prefix website ci
npm --prefix website run dev
```

Open http://127.0.0.1:5178/. `website/start.cmd` or
`bash website/start.sh` installs missing dependencies and opens the preview.

```sh
npm --prefix website run check
npm --prefix website run build
npm --prefix website run preview
```

Only `website/dist/` is published. `public/` holds site-only assets and crawler
files. `scripts/prepare-assets.mjs` copies tracked shared application/README
images from root `assets/` into the build inputs; runtime uploads are excluded.
Those generated image copies are ignored by Git. Keep application assets at the
root because the app and release docs use them independently of the website.

To verify a project-path deployment in PowerShell:

```powershell
$env:SITE_BASE_PATH = '/server-domme/'
npm --prefix website run build
Remove-Item Env:SITE_BASE_PATH
```

The build verifies local links, anchors and the published HTML allow-list.

## GitHub Pages

The Website workflow checks pull requests and publishes `website/dist/` on
`main`. It uses Node.js 24 and the npm lockfile. In Settings → Pages, use
**GitHub Actions** as the source. The custom domain is `server-domme.keshon.ru`;
`public/CNAME` preserves it in the output. Keep the domain and HTTPS configured
in the repository's Pages settings.

This repository previously published the root of `main`. Switch Pages to
GitHub Actions when deploying the new layout. A branch-root build no longer
contains the homepage.
