import { DEFAULT_PORT, resolvePort } from '../src/port';

describe('resolvePort', () => {
  it('falls back to the default, silently, when PORT is unset or empty', () => {
    expect(resolvePort(undefined)).toEqual({ port: DEFAULT_PORT });
    expect(resolvePort('')).toEqual({ port: DEFAULT_PORT });
  });

  it('accepts one to five ASCII digits within 1..65535', () => {
    expect(resolvePort('8080')).toEqual({ port: 8080 });
    expect(resolvePort('1')).toEqual({ port: 1 });
    expect(resolvePort('65535')).toEqual({ port: 65535 });
  });

  it('rejects anything else and reports the raw value', () => {
    for (const raw of [
      '0',
      '65536',
      'abc',
      '80a',
      ' 3000',
      '1e3',
      '0x1F',
      '-1',
      '3000.0',
    ]) {
      expect(resolvePort(raw)).toEqual({ port: DEFAULT_PORT, rejected: raw });
    }
  });
});
