import { Module } from '@nestjs/common';
import { DocsController } from './docs.controller';
import { TenantDiscoveryService } from './tenant-discovery.service';
import { MarkdownRendererService } from './markdown-renderer.service';
import { YamlHighlighterService } from './yaml-highlighter.service';
import { ExportService } from './export/export.service';
import { DriftService } from './drift.service';

@Module({
  controllers: [DocsController],
  providers: [
    TenantDiscoveryService,
    MarkdownRendererService,
    YamlHighlighterService,
    ExportService,
    DriftService,
  ],
})
export class DocsModule {}
