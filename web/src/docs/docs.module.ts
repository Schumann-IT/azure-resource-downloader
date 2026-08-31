import { Module } from '@nestjs/common';
import { DocsController } from './docs.controller';
import { TenantDiscoveryService } from './tenant-discovery.service';
import { MarkdownRendererService } from './markdown-renderer.service';
import { YamlHighlighterService } from './yaml-highlighter.service';

@Module({
  controllers: [DocsController],
  providers: [
    TenantDiscoveryService,
    MarkdownRendererService,
    YamlHighlighterService,
  ],
})
export class DocsModule {}
