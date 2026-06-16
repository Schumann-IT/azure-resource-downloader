import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { Logger } from '@nestjs/common';
import { AppModule } from './app.module';
import { API_PREFIX, resolveOutputDir, resolvePort } from './config';

async function bootstrap() {
  const app = await NestFactory.create(AppModule, { cors: true });
  app.setGlobalPrefix(API_PREFIX);
  const port = resolvePort();
  await app.listen(port);
  const log = new Logger('bootstrap');
  log.log(`API listening on http://localhost:${port}/${API_PREFIX}`);
  log.log(`Serving output tree from ${resolveOutputDir()}`);
}

void bootstrap();
