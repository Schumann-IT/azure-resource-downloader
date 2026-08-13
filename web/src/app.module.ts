import { Module } from '@nestjs/common';
import { DocsModule } from './docs/docs.module';

@Module({
  imports: [DocsModule],
})
export class AppModule {}
