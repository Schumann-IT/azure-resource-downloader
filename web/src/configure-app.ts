import { NestExpressApplication } from '@nestjs/platform-express';
import { join } from 'path';

// hbs is a CommonJS singleton; require it directly so registerPartials is bound.
// eslint-disable-next-line @typescript-eslint/no-var-requires
const hbs = require('hbs');

// Wires the Handlebars view engine and static assets. Shared by the runtime
// bootstrap (main.ts) and the e2e tests so both configure the app identically.
export function configureViews(app: NestExpressApplication): void {
  const viewsDir = join(process.cwd(), 'views');
  const publicDir = join(process.cwd(), 'public');

  app.useStaticAssets(publicDir);
  app.setBaseViewsDir(viewsDir);
  hbs.registerPartials(join(viewsDir, 'partials'));
  app.setViewEngine('hbs');
}
