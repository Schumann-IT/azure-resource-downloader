export const DEFAULT_PORT = 3000;

// `Number()` accepts things an operator never means by a port — `" 3000 "`,
// `"1e3"`, `"0x1F"` — and `PORT=0` asks the OS for a random port, so parsing
// is a strict digit pattern plus a range check rather than a cast. Pure and
// synchronous so it is unit testable without booting the app (`main.ts`
// listens and cannot be).
export function resolvePort(raw: string | undefined): {
  port: number;
  rejected?: string;
} {
  if (raw === undefined || raw === '') {
    return { port: DEFAULT_PORT };
  }
  if (/^\d{1,5}$/.test(raw)) {
    const value = Number(raw);
    if (value >= 1 && value <= 65535) {
      return { port: value };
    }
  }
  return { port: DEFAULT_PORT, rejected: raw };
}
