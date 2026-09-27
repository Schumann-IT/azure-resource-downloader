import { rewriteHref } from '../src/docs/link-rewrite';

describe('rewriteHref without a route base (documentation)', () => {
  const env = { tenant: 't', docDir: 'Microsoft.Graph/policies' };

  it('resolves a relative .md link against the document directory', () => {
    expect(rewriteHref('../groups/g1.md', env)).toBe('/t/Microsoft.Graph/groups/g1');
    expect(rewriteHref('p2.md#settings', env)).toBe(
      '/t/Microsoft.Graph/policies/p2#settings',
    );
  });

  it('resolves a docs-root link from the root', () => {
    expect(
      rewriteHref('Microsoft.Graph/groups/g1.md', { tenant: 't', docDir: '' }),
    ).toBe('/t/Microsoft.Graph/groups/g1');
  });

  it('leaves anchors, absolute routes, schemes and non-.md targets untouched', () => {
    for (const href of [
      '#x',
      '/t/abs',
      '//host/x.md',
      'https://example.com/x.md',
      'mailto:a@b',
      '../groups/g1.yaml',
      '',
    ]) {
      expect(rewriteHref(href, env)).toBeNull();
    }
  });

  it('leaves a link escaping the tenant root untouched, docs/ included', () => {
    expect(rewriteHref('../../../x.md', env)).toBeNull();
    expect(rewriteHref('../../../docs/Microsoft.Graph/groups/g1.md', env)).toBeNull();
  });
});

describe('rewriteHref with the drift route base', () => {
  const env = {
    tenant: 't',
    docDir: 'Microsoft.Graph/deviceConfigurations',
    routeBase: '_drift',
  };

  it('keeps links between drift documents inside the drift view', () => {
    expect(rewriteHref('other.md', env)).toBe(
      '/t/_drift/Microsoft.Graph/deviceConfigurations/other',
    );
    expect(rewriteHref('../groups/g1.md#who', env)).toBe(
      '/t/_drift/Microsoft.Graph/groups/g1#who',
    );
  });

  it('resolves the drift index links from the drift root', () => {
    expect(
      rewriteHref('Microsoft.Graph/deviceConfigurations/c1.md', {
        tenant: 't',
        docDir: '',
        routeBase: '_drift',
      }),
    ).toBe('/t/_drift/Microsoft.Graph/deviceConfigurations/c1');
  });

  it('turns a link into the sibling docs/ tree into the documentation route', () => {
    expect(
      rewriteHref('../../../docs/Microsoft.Graph/deviceConfigurations/c1.md#settings', env),
    ).toBe('/t/Microsoft.Graph/deviceConfigurations/c1#settings');
  });

  it('leaves any other escape untouched', () => {
    expect(rewriteHref('../../../resources/x.md', env)).toBeNull();
    expect(rewriteHref('../../../../docs/x.md', env)).toBeNull();
    expect(rewriteHref('../../../docs/.md', env)).toBeNull();
  });
});
