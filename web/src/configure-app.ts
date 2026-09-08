import { NestExpressApplication } from '@nestjs/platform-express';
import { NextFunction, Request, Response } from 'express';
import { join } from 'path';

// hbs is a CommonJS singleton; require it directly so registerPartials is bound.
// eslint-disable-next-line @typescript-eslint/no-require-imports
const hbs = require('hbs');

// Baseline hardening headers, sent on every response. The CSP is derived from
// what the pages actually load, not copied from a template: `style-src`
// carries 'unsafe-inline' because shiki (defaultColor: false) emits inline
// --shiki-light/--shiki-dark custom properties on every YAML token; `img-src`
// carries data: because the severity/section icons are data: SVGs applied
// through a CSS mask (a mask fetch is an image load) and https: for images a
// document embeds. Nothing else is loaded (no fonts, fetch, frames, forms,
// media or workers), so `default-src 'none'` is safe and leaves `script-src`
// unnamed — i.e. none. `X-Frame-Options` duplicates `frame-ancestors` for
// browsers that predate it.
export const SECURITY_HEADERS: Readonly<Record<string, string>> = {
  'Content-Security-Policy':
    "default-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; frame-ancestors 'none'; base-uri 'none'",
  'X-Content-Type-Options': 'nosniff',
  'Referrer-Policy': 'same-origin',
  'X-Frame-Options': 'DENY',
};

// Wires the Handlebars view engine, static assets and the security headers.
// Shared by the runtime bootstrap (main.ts) and the e2e tests so both
// configure the app identically.
export function configureViews(app: NestExpressApplication): void {
  const viewsDir = join(process.cwd(), 'views');
  const publicDir = join(process.cwd(), 'public');

  // First, so static assets (favicon, app.css) carry the headers too.
  app.use((_req: Request, res: Response, next: NextFunction) => {
    for (const [name, value] of Object.entries(SECURITY_HEADERS)) {
      res.setHeader(name, value);
    }
    next();
  });
  app.useStaticAssets(publicDir);
  app.setBaseViewsDir(viewsDir);
  hbs.registerPartials(join(viewsDir, 'partials'));
  app.setViewEngine('hbs');
}
