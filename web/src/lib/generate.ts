// Random values for secret variables. Everything here avoids characters that
// break .env files, shells or compose interpolation: no $, quotes,
// backslashes, spaces or #.

export type SecretKind = "alnum" | "hex" | "base64";

export interface SecretFormat {
  kind: SecretKind;
  // Characters in the result; for base64 the count of random bytes instead
  length: number;
}

const alphabets = {
  alnum: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789",
  hex: "0123456789abcdef",
};

// Random integer below max without modulo bias: values from the biased tail
// of the 32-bit range are thrown away and drawn again
function randomBelow(max: number): number {
  const limit = Math.floor(0x100000000 / max) * max;
  const buf = new Uint32Array(1);
  do {
    crypto.getRandomValues(buf);
  } while (buf[0] >= limit);
  return buf[0] % max;
}

function randomChars(alphabet: string, length: number): string {
  let out = "";
  for (let i = 0; i < length; i++) out += alphabet[randomBelow(alphabet.length)];
  return out;
}

function randomBase64(bytes: number): string {
  const buf = new Uint8Array(bytes);
  crypto.getRandomValues(buf);
  let bin = "";
  for (const b of buf) bin += String.fromCharCode(b);
  return btoa(bin);
}

export function generateSecret(format: SecretFormat): string {
  if (format.kind === "base64") return randomBase64(format.length);
  return randomChars(alphabets[format.kind], format.length);
}

// Picks a format from what the stack says about the variable. Longer values
// go to names that look like application signing keys (Django's SECRET_KEY,
// Sentry's SENTRY_SECRET_KEY); the rest get 32 characters.
export function secretFormat(key: string, description = "", example = ""): SecretFormat {
  const longKey = /SECRET_KEY|(^|_)KEY$/i.test(key);
  const sample = example.trim();

  if (/base-?64/i.test(description) || /base64/i.test(key)) {
    return { kind: "base64", length: 32 };
  }
  // An example only counts as hex when it is long enough to be a token and
  // has a letter, so a numeric default like "12345678" doesn't
  const sampleHex = /^[0-9a-f]{16,}$/i.test(sample) && /[a-f]/i.test(sample);
  if (/\bhex(adecimal)?\b/i.test(description) || /(^|_)HEX(_|$)/i.test(key) || sampleHex) {
    const length = sampleHex ? Math.min(sample.length, 128) : longKey ? 64 : 32;
    return { kind: "hex", length };
  }
  return { kind: "alnum", length: longKey ? 50 : 32 };
}
