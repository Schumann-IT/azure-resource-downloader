import { Module } from '@nestjs/common';
import { DocsController } from './docs.controller';
import { TenantDiscoveryService } from './tenant-discovery.service';
import { MarkdownRendererService } from './markdown-renderer.service';

@Module({
  controllers: [DocsController],
  providers: [TenantDiscoveryService, MarkdownRendererService],
})
export class DocsModule {}
