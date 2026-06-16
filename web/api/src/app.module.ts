import { Module } from '@nestjs/common';
import { OutputModule } from './output/output.module';

@Module({
  imports: [OutputModule],
})
export class AppModule {}
