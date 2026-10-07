// The hero "generates" a headline the way a language model would: token by
// token, each with a probability, attending back over what came before. The
// numbers are a deterministic stage effect, not a model, but the shapes are
// the real ones: causal masking, an attention sink on the first token,
// recency bias, and temperature-weighted sampling.

const MAGMA: [number, [number, number, number]][] = [
  [0.0, [7, 6, 11]],
  [0.2, [59, 15, 112]],
  [0.4, [140, 41, 129]],
  [0.6, [222, 73, 104]],
  [0.8, [254, 159, 109]],
  [1.0, [252, 253, 191]],
];

/** Colour for v in [0, 1] on the magma ramp. */
export function magma(v: number): string {
  const x = Math.min(1, Math.max(0, v));
  for (let i = 1; i < MAGMA.length; i++) {
    const [t1, c1] = MAGMA[i];
    if (x <= t1) {
      const [t0, c0] = MAGMA[i - 1];
      const f = (x - t0) / (t1 - t0);
      const c = c0.map((a, k) => Math.round(a + (c1[k] - a) * f));
      return `rgb(${c[0]} ${c[1]} ${c[2]})`;
    }
  }
  return 'rgb(252 253 191)';
}

export interface Token {
  text: string; // including any trailing space
  word: string; // normalized, for comparison
  p: number;
  alternatives: { text: string; p: number }[];
}

/** Split into word-ish tokens, punctuation attached to the following space. */
export function tokenize(s: string): string[] {
  return s.match(/[\p{L}\p{N}'’·./-]+[,:;.!?)]*\s*|[^\s\p{L}\p{N}]+\s*/gu) ?? [s];
}

function norm(t: string) {
  return t.trim().toLowerCase().replace(/[,:;.!?)]+$/, '');
}

/** Small stable hash → [0, 1). Same input, same "probability". */
function unit(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return ((h >>> 0) % 10000) / 10000;
}

/**
 * Builds the token stream for headlines[pick]. Tokens that agree with the
 * greedy answer (headlines[0]) at the same position are confident; tokens
 * where the sample diverged are not. Alternatives come from what the other
 * headlines say at that position.
 */
export function buildTokens(headlines: string[], pick: number): Token[] {
  const all = headlines.map(tokenize);
  const greedy = all[0] ?? [];
  const chosen = all[pick] ?? greedy;

  return chosen.map((text, i) => {
    const word = norm(text);
    const agrees = norm(greedy[i] ?? '') === word;
    const p = agrees ? 0.78 + unit(text + i) * 0.21 : 0.18 + unit(text + i) * 0.4;

    const seen = new Set([word]);
    const alts: string[] = [];
    for (const toks of all) {
      const t = toks[i];
      if (t && !seen.has(norm(t))) {
        seen.add(norm(t));
        alts.push(t.trim());
      }
    }
    let rest = 1 - p;
    const alternatives = alts.slice(0, 3).map((t, k) => {
      const share = rest * (0.55 - k * 0.1);
      rest -= share;
      return { text: t, p: share };
    });
    return { text, word, p, alternatives };
  });
}

const STOP = new Set(['a', 'an', 'the', 'and', 'or', 'to', 'of', 'in', 'on', 'at', 'for', 'from', 'that', 'i', 'it', 'is', 'by', 'with', 'then', 'now', 'where', 'their']);

/**
 * Row-normalised causal attention. Row i is how much token i attends to each
 * token j ≤ i: a sink on token 0, decay with distance, and a pull between
 * content words that share a stem.
 */
export function attention(tokens: Token[]): number[][] {
  return tokens.map((ti, i) => {
    const row = tokens.map((tj, j) => {
      if (j > i) return 0;
      let s = Math.exp(-(i - j) / 2.2);
      if (j === 0) s += 0.55;
      if (i !== j && !STOP.has(ti.word) && !STOP.has(tj.word)) {
        if (ti.word.slice(0, 3) === tj.word.slice(0, 3)) s += 0.5;
        s += 0.12 * unit(ti.word + '|' + tj.word);
      }
      return s;
    });
    const max = Math.max(...row);
    return row.map((v) => v / max);
  });
}

/**
 * Picks a headline index. T = 0 is greedy (always 0). Higher T flattens the
 * distribution over the alternatives, the way temperature does over logits.
 */
export function sample(count: number, temperature: number, avoid?: number): number {
  if (count <= 1 || temperature <= 0) return 0;
  const logits = Array.from({ length: count }, (_, i) => -i * 1.2);
  const weights = logits.map((l, i) => (i === avoid ? 0 : Math.exp(l / temperature)));
  const total = weights.reduce((a, b) => a + b, 0);
  if (total === 0) return 0;
  let r = Math.random() * total;
  for (let i = 0; i < count; i++) {
    r -= weights[i];
    if (r <= 0) return i;
  }
  return count - 1;
}
