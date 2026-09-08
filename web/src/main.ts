import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { NestExpressApplication } from '@nestjs/platform-express';
import { AppModule } from './app.module';
import { configureViews } from './configure-app';
import { resolvePort } from './port';

async function bootstrap(): Promise<void> {
  const app = await NestFactory.create<NestExpressApplication>(AppModule);

  // Views and static assets are resolved from the project root (web/), so they
  // work identically in dev (`nest start`) and prod (`node dist/main.js`).
  configureViews(app);

  const { port, rejected } = resolvePort(process.env.PORT);
  await app.listen(port);
  const note = rejected
    ? ` (PORT=${JSON.stringify(rejected)} is not a port in 1..65535, using ${port})`
    : '';
  // eslint-disable-next-line no-console
  console.log(`Docs browser listening on http://localhost:${port}${note}`);
}

void bootstrap();
