import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { NestExpressApplication } from '@nestjs/platform-express';
import { AppModule } from './app.module';
import { configureViews } from './configure-app';

async function bootstrap(): Promise<void> {
  const app = await NestFactory.create<NestExpressApplication>(AppModule);

  // Views and static assets are resolved from the project root (web/), so they
  // work identically in dev (`nest start`) and prod (`node dist/main.js`).
  configureViews(app);

  const port = process.env.PORT ? Number(process.env.PORT) : 3000;
  await app.listen(port);
  // eslint-disable-next-line no-console
  console.log(`Docs browser listening on http://localhost:${port}`);
}

void bootstrap();
