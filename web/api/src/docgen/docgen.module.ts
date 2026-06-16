import { Module } from '@nestjs/common';
import { DocgenCommand } from './docgen.command';
import { DocgenService } from './docgen.service';

@Module({
  providers: [DocgenService, DocgenCommand],
  exports: [DocgenService],
})
export class DocgenModule {}
