/**
 * md4go E2E tests — Node.js native test runner (zero dependencies).
 *
 * Prerequisites:
 *   1. Build WASM:  GOOS=js GOARCH=wasm go build -o md4go.wasm ./wasm
 *   2. Copy runtime: cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" wasm/
 *
 * Usage:
 *   node --test --test-reporter spec wasm/md4go_e2e.test.js
 *   # Node 18: node --experimental-test-runner wasm/md4go_e2e.test.js
 */

import { createRequire } from 'node:module';
import { describe, it, before } from 'node:test';
import assert from 'node:assert/strict';

// ── wasm_exec.js needs require() in Node.js ESM ──────────────────────
// Must be set BEFORE wasm_exec.js is loaded (dynamic import in before hook).
globalThis.require = createRequire(import.meta.url);

let md4go;

// ── Node.js fetch polyfill for file:// URLs ─────────────────────────
// Node.js global fetch does not support file:// protocol.
// Monkey-patch before wasm_exec.js or md4go.js are loaded.
{
  const _fetch = globalThis.fetch;
  globalThis.fetch = async (url, ...args) => {
    if (typeof url === 'string' && url.startsWith('file://')) {
      const { readFile } = await import('node:fs/promises');
      const buf = await readFile(new URL(url));
      return new Response(buf, { headers: { 'Content-Type': 'application/wasm' } });
    }
    return _fetch(url, ...args);
  };
}

before(async () => {
  await import('./wasm_exec.js');            // defines globalThis.Go
  const mod = await import('./md4go.js');    // initMd4go
  // Node.js fetch requires a full URL; resolve relative path to file://
  const wasmUrl = new URL('./md4go.wasm', import.meta.url).href;
  md4go = await mod.initMd4go(wasmUrl);
});

// ═══════════════════════════════════════════════════════════════════════
// parseToHTML
// ═══════════════════════════════════════════════════════════════════════

describe('parseToHTML', () => {
  it('heading', () => {
    const got = md4go.parseToHTML('# Hello');
    assert.ok(got.includes('<h1>Hello</h1>'), `got: ${got}`);
  });

  it('emphasis and strong', () => {
    const got = md4go.parseToHTML('**bold** *italic*');
    assert.ok(got.includes('<strong>bold</strong>'), `got: ${got}`);
    assert.ok(got.includes('<em>italic</em>'), `got: ${got}`);
  });

  it('link', () => {
    const got = md4go.parseToHTML('[md4go](https://github.com/userpro/md4go)');
    assert.ok(got.includes('<a href="https://github.com/userpro/md4go">md4go</a>'), got);
  });

  it('code span and fenced code', () => {
    const got = md4go.parseToHTML('`inline`\n\n```go\nfunc main() {}\n```');
    assert.ok(got.includes('<code>inline</code>'), got);
    assert.ok(got.includes('<code class="language-go">'), got);
  });

  it('unordered list', () => {
    const got = md4go.parseToHTML('- a\n- b\n- c');
    assert.ok(got.includes('<ul>'), got);
    assert.ok(got.includes('<li>a</li>'), got);
    assert.ok(got.includes('<li>b</li>'), got);
    assert.ok(got.includes('<li>c</li>'), got);
  });

  it('GFM table', () => {
    const md = '| a | b |\n|---|---|\n| 1 | 2 |';
    const got = md4go.parseToHTML(md);
    assert.ok(got.includes('<table>'), got);
    assert.ok(got.includes('<th>a</th>'), got);
    assert.ok(got.includes('<td>1</td>'), got);
  });

  it('GFM strikethrough', () => {
    const got = md4go.parseToHTML('~~deleted~~');
    assert.ok(got.includes('<del>deleted</del>'), got);
  });

  it('GFM task list', () => {
    const got = md4go.parseToHTML('- [x] done\n- [ ] todo');
    // GFM task lists render checkboxes
    assert.ok(got.includes('checkbox'), `expected checkbox in: ${got}`);
  });

  it('XHTML self-closing br (default)', () => {
    // Hard break creates <br />
    const got = md4go.parseToHTML('line1  \nline2');
    assert.ok(got.includes('<br />'), `got: ${got}`);
  });

  it('HTML br without XHTML via options object', () => {
    const got = md4go.parseToHTML('line1  \nline2', {
      flags: md4go.Flags.GitHub,
      rendererFlags: 0, // no FlagXHTML
    });
    assert.ok(got.includes('<br>'), `got: ${got}`);
  });

  it('CommonMark mode (no GFM)', () => {
    const got = md4go.parseToHTML('~~text~~', md4go.Flags.CommonMark);
    // In CommonMark mode, ~~ is NOT strikethrough
    assert.ok(!got.includes('<del>'), `got: ${got}`);
  });

  it('empty string', () => {
    // Empty input produces a bare newline; verify no HTML tags.
    const got = md4go.parseToHTML('');
    assert.ok(!got.includes('<'), `unexpected HTML tag in: ${got}`);
  });

  it('unicode CJK', () => {
    const got = md4go.parseToHTML('## 你好世界');
    assert.ok(got.includes('你好世界'), got);
  });
});

// ═══════════════════════════════════════════════════════════════════════
// parseToText
// ═══════════════════════════════════════════════════════════════════════

describe('parseToText', () => {
  it('plain paragraph', () => {
    const got = md4go.parseToText('hello world');
    assert.ok(got.includes('hello world'), got);
  });

  it('heading text only', () => {
    const got = md4go.parseToText('# Title');
    assert.ok(got.includes('Title'), got);
    assert.ok(!got.includes('#'), `should not contain markdown syntax: ${got}`);
  });

  it('link strips to text', () => {
    const got = md4go.parseToText('[click](https://x.com)');
    assert.ok(got.includes('click'), got);
    assert.ok(!got.includes('https://x.com'), `should strip URL: ${got}`);
  });

  it('empty string', () => {
    // Empty input produces a bare newline; verify no HTML or markdown syntax.
    const got = md4go.parseToText('');
    assert.ok(got.length <= 1, `unexpected content: ${got}`);
    assert.ok(!got.includes('#'), `unexpected markdown: ${got}`);
  });
});

// ═══════════════════════════════════════════════════════════════════════
// parseToHTMLWithOptions
// ═══════════════════════════════════════════════════════════════════════

describe('parseToHTMLWithOptions', () => {
  it('flags + rendererFlags combined', () => {
    const md = 'line1  \nline2';
    const got = md4go.parseToHTMLWithOptions(md, {
      flags: md4go.Flags.CommonMark,
      rendererFlags: 0,
    });
    assert.ok(got.includes('<br>'), `should be non-XHTML: ${got}`);
    assert.ok(!got.includes('<br />'), `should not be XHTML: ${got}`);
  });

  it('defaults when no options', () => {
    const got = md4go.parseToHTMLWithOptions('# Hi');
    assert.ok(got.includes('<h1>Hi</h1>'), got);
  });

  it('partial options (only flags)', () => {
    const got = md4go.parseToHTMLWithOptions('~~x~~', { flags: 0 });
    assert.ok(!got.includes('<del>'), `CommonMark mode should not have del: ${got}`);
  });
});

// ═══════════════════════════════════════════════════════════════════════
// parseWithRenderer
// ═══════════════════════════════════════════════════════════════════════

describe('parseWithRenderer', () => {
  it('extract links', () => {
    const links = [];
    const result = md4go.parseWithRenderer(
      '[a](https://a.com) and [b](https://b.org "title")',
      {
        enterSpan(type, detail) {
          if (type === md4go.SpanType.SpanLink) {
            links.push({ href: detail.href, title: detail.title });
          }
        },
      }
    );
    assert.equal(result, null);
    assert.equal(links.length, 2);
    assert.equal(links[0].href, 'https://a.com');
    assert.equal(links[0].title, '');
    assert.equal(links[1].href, 'https://b.org');
    assert.equal(links[1].title, 'title');
  });

  it('build outline from headings', () => {
    const outline = [];
    md4go.parseWithRenderer('# Intro\n## Details\n# End', {
      enterBlock(type, detail) {
        if (type === md4go.BlockType.BlockH) {
          outline.push({ level: detail.level, text: '' });
        }
      },
      text(type, text) {
        if (type === md4go.TextType.TextNormal && outline.length > 0) {
          outline[outline.length - 1].text += text;
        }
      },
    });
    assert.equal(outline.length, 3);
    assert.deepEqual(outline[0], { level: 1, text: 'Intro' });
    assert.deepEqual(outline[1], { level: 2, text: 'Details' });
    assert.deepEqual(outline[2], { level: 1, text: 'End' });
  });

  it('count blocks and spans', () => {
    const counts = { blocks: 0, spans: 0, chars: 0 };
    md4go.parseWithRenderer('**bold** and *italic*', {
      enterBlock() { counts.blocks++; },
      enterSpan()  { counts.spans++; },
      text(type, text) {
        if (type === md4go.TextType.TextNormal) counts.chars += text.length;
      },
    });
    assert.ok(counts.blocks > 0, 'should have blocks');
    assert.ok(counts.spans >= 2, 'should have at least 2 spans');
    assert.ok(counts.chars > 0, 'should have characters');
  });

  it('all callbacks optional', () => {
    const result = md4go.parseWithRenderer('# Hi', {});
    assert.equal(result, null);
  });

  it('single callback only', () => {
    let called = false;
    md4go.parseWithRenderer('text', {
      text() { called = true; },
    });
    assert.ok(called);
  });

  it('GFM strikethrough via renderer', () => {
    const dels = [];
    md4go.parseWithRenderer('~~deleted~~ text', {
      enterSpan(type, detail) {
        if (type === md4go.SpanType.SpanDel) {
          dels.push('del');
        }
      },
    });
    assert.equal(dels.length, 1, 'should see SpanDel with default GFM flags');
  });

  it('explicit flags (3-arg form)', () => {
    const dels = [];
    md4go.parseWithRenderer('~~text~~', md4go.Flags.CommonMark, {
      enterSpan(type) {
        if (type === md4go.SpanType.SpanDel) dels.push('del');
      },
    });
    assert.equal(dels.length, 0, 'no SpanDel in CommonMark mode');
  });
});

// ═══════════════════════════════════════════════════════════════════════
// createParser (reusable parser)
// ═══════════════════════════════════════════════════════════════════════

describe('createParser', () => {
  it('reusable: multiple parseToHTML', () => {
    const p = md4go.createParser(md4go.Flags.GitHub);
    const a = p.parseToHTML('# First');
    const b = p.parseToHTML('## Second');
    assert.ok(a.includes('<h1>First</h1>'), a);
    assert.ok(b.includes('<h2>Second</h2>'), b);
    p.dispose();
  });

  it('parseToText on parser', () => {
    const p = md4go.createParser();
    const got = p.parseToText('**bold** text');
    assert.ok(got.includes('bold'), got);
    assert.ok(!got.includes('**'), got);
    p.dispose();
  });

  it('parseWithRenderer on parser', () => {
    const p = md4go.createParser();
    const links = [];
    const result = p.parseWithRenderer('[x](y)', {
      enterSpan(type, detail) {
        if (type === md4go.SpanType.SpanLink) links.push(detail.href);
      },
    });
    assert.equal(result, null);
    assert.equal(links.length, 1);
    assert.equal(links[0], 'y');
    p.dispose();
  });

  it('parser with custom renderer flags', () => {
    const p = md4go.createParser();
    // Non-XHTML
    const got = p.parseToHTML('a  \nb', 0);
    assert.ok(got.includes('<br>'), `got: ${got}`);
    assert.ok(!got.includes('<br />'), `should not be XHTML: ${got}`);
    p.dispose();
  });

  it('dispose does not throw', () => {
    const p = md4go.createParser();
    p.parseToHTML('# x');
    assert.doesNotThrow(() => p.dispose());
  });

  it('parseWithRenderer respects parser creation flags', () => {
    // Parser created with CommonMark: ~~x~~ should NOT produce SpanDel.
    const p = md4go.createParser(md4go.Flags.CommonMark);
    const dels = [];
    p.parseWithRenderer('~~x~~', {
      enterSpan(type) {
        if (type === md4go.SpanType.SpanDel) dels.push('del');
      },
    });
    assert.equal(dels.length, 0, 'CommonMark parser should not emit SpanDel');
    p.dispose();
  });
});

// ═══════════════════════════════════════════════════════════════════════
// createStreamParser (chunked)
// ═══════════════════════════════════════════════════════════════════════

describe('createStreamParser', () => {
  it('write chunks then finishHTML', () => {
    const s = md4go.createStreamParser();
    s.write('# Part 1\n\n');
    s.write('## Part 2\n');
    const html = s.finishHTML();
    assert.ok(html.includes('<h1>Part 1</h1>'), html);
    assert.ok(html.includes('<h2>Part 2</h2>'), html);
    s.dispose();
  });

  it('finishText after chunks', () => {
    const s = md4go.createStreamParser();
    s.write('hello ');
    s.write('world');
    const text = s.finishText();
    assert.ok(text.includes('hello'), text);
    assert.ok(text.includes('world'), text);
    s.dispose();
  });

  it('empty stream returns empty', () => {
    const s = md4go.createStreamParser();
    // Empty input produces a bare newline; verify no HTML or text content.
    const html = s.finishHTML();
    const text = s.finishText();
    assert.ok(!html.includes('<'), `unexpected HTML in: ${html}`);
    assert.ok(text.length <= 1, `unexpected text content: ${text}`);
    s.dispose();
  });

  it('buffer resets after finish', () => {
    const s = md4go.createStreamParser();
    s.write('hello');
    const first = s.finishText();
    assert.ok(first.includes('hello'), first);

    // After finish, buffer should be reset
    s.write('world');
    const second = s.finishText();
    assert.ok(second.includes('world'), second);
    assert.ok(!second.includes('hello'), `should not contain previous: ${second}`);
    s.dispose();
  });

  it('finishHTML with renderer flags', () => {
    const s = md4go.createStreamParser();
    s.write('a  \nb');
    const html = s.finishHTML(0); // non-XHTML
    assert.ok(html.includes('<br>'), html);
    assert.ok(!html.includes('<br />'), html);
    s.dispose();
  });

  it('single write large text', () => {
    const s = md4go.createStreamParser();
    const big = '# Big\n\n' + 'paragraph '.repeat(500);
    s.write(big);
    const html = s.finishHTML();
    assert.ok(html.includes('<h1>Big</h1>'), `got: ${html.substring(0, 200)}`);
    assert.ok(html.length > big.length, 'HTML output should be larger than input');
    s.dispose();
  });

  it('explicit CommonMark flags', () => {
    const s = md4go.createStreamParser(md4go.Flags.CommonMark);
    s.write('~~deleted~~');
    const html = s.finishHTML();
    // CommonMark mode → no <del>
    assert.ok(!html.includes('<del>'), `CommonMark should not have del: ${html}`);
    s.dispose();
  });

  it('finishWithRenderer: extract links from streamed input', () => {
    const s = md4go.createStreamParser();
    s.write('[a](https://a.com)');
    s.write(' and [b](https://b.org "title")');
    const links = [];
    const result = s.finishWithRenderer({
      enterSpan(type, detail) {
        if (type === md4go.SpanType.SpanLink) {
          links.push({ href: detail.href, title: detail.title });
        }
      },
    });
    assert.equal(result, null);
    assert.equal(links.length, 2);
    assert.equal(links[0].href, 'https://a.com');
    assert.equal(links[0].title, '');
    assert.equal(links[1].href, 'https://b.org');
    assert.equal(links[1].title, 'title');
    s.dispose();
  });

  it('finishWithRenderer: build outline from streamed headings', () => {
    const s = md4go.createStreamParser();
    s.write('# Intro\n');
    s.write('## Details\n');
    s.write('# End');
    const outline = [];
    s.finishWithRenderer({
      enterBlock(type, detail) {
        if (type === md4go.BlockType.BlockH) {
          outline.push({ level: detail.level, text: '' });
        }
      },
      text(type, text) {
        if (type === md4go.TextType.TextNormal && outline.length > 0) {
          outline[outline.length - 1].text += text;
        }
      },
    });
    assert.equal(outline.length, 3);
    assert.deepEqual(outline[0], { level: 1, text: 'Intro' });
    assert.deepEqual(outline[1], { level: 2, text: 'Details' });
    assert.deepEqual(outline[2], { level: 1, text: 'End' });
    s.dispose();
  });

  it('finishWithRenderer: reset buffer after finish', () => {
    const s = md4go.createStreamParser();
    s.write('[first](https://a.com)');
    const links1 = [];
    s.finishWithRenderer({
      enterSpan(type, detail) {
        if (type === md4go.SpanType.SpanLink) links1.push(detail.href);
      },
    });
    assert.deepEqual(links1, ['https://a.com']);

    // Buffer should be reset — new write is independent
    s.write('[second](https://b.com)');
    const links2 = [];
    s.finishWithRenderer({
      enterSpan(type, detail) {
        if (type === md4go.SpanType.SpanLink) links2.push(detail.href);
      },
    });
    assert.deepEqual(links2, ['https://b.com']);
    s.dispose();
  });

  it('finishWithRenderer: CommonMark flags respected', () => {
    const s = md4go.createStreamParser(md4go.Flags.CommonMark);
    s.write('~~deleted~~');
    const dels = [];
    s.finishWithRenderer({
      enterSpan(type) {
        if (type === md4go.SpanType.SpanDel) dels.push('del');
      },
    });
    assert.equal(dels.length, 0, 'CommonMark stream should not emit SpanDel');
    s.dispose();
  });

  it('finishWithRenderer: empty stream', () => {
    const s = md4go.createStreamParser();
    const counts = { blocks: 0, spans: 0 };
    const result = s.finishWithRenderer({
      enterBlock() { counts.blocks++; },
      enterSpan()  { counts.spans++; },
    });
    assert.equal(result, null);
    // Empty input emits BlockDoc enter (and leave, which we don't count)
    assert.ok(counts.blocks >= 1, `expected at least Doc enter: ${counts.blocks}`);
    s.dispose();
  });
});

// ═══════════════════════════════════════════════════════════════════════
// Constants
// ═══════════════════════════════════════════════════════════════════════

describe('Constants', () => {
  it('BlockType values', () => {
    const B = md4go.BlockType;
    assert.equal(B.BlockDoc, 0);
    assert.equal(B.BlockQuote, 1);
    assert.equal(B.BlockUL, 2);
    assert.equal(B.BlockOL, 3);
    assert.equal(B.BlockLI, 4);
    assert.equal(B.BlockH, 6);
    assert.equal(B.BlockCode, 7);
    assert.equal(B.BlockP, 9);
    assert.equal(B.BlockTable, 10);
    assert.equal(B.BlockTH, 14);
    assert.equal(B.BlockTD, 15);
    assert.equal(B.BlockFootnoteDef, 17);
    assert.equal(B.BlockAdmonition, 18);
  });

  it('SpanType values', () => {
    const S = md4go.SpanType;
    assert.equal(S.SpanEm, 0);
    assert.equal(S.SpanStrong, 1);
    assert.equal(S.SpanLink, 2);
    assert.equal(S.SpanImg, 3);
    assert.equal(S.SpanCode, 4);
    assert.equal(S.SpanDel, 5);
    assert.equal(S.SpanWikilink, 8);
    assert.equal(S.SpanFootnoteRef, 13);
  });

  it('TextType values', () => {
    const T = md4go.TextType;
    assert.equal(T.TextNormal, 0);
    assert.equal(T.TextBR, 2);
    assert.equal(T.TextEntity, 4);
    assert.equal(T.TextCode, 5);
    assert.equal(T.TextHTML, 6);
  });

  it('Flags presets', () => {
    assert.equal(md4go.Flags.CommonMark, 0);
    // DialectGitHub = PermissiveAutolinks|Tables|Strikethrough|Tasklists|Admonitions|Footnotes
    assert.ok(md4go.Flags.GitHub > 0, 'GitHub flags should be non-zero');
    // Verify it differs from CommonMark
    assert.notEqual(md4go.Flags.GitHub, md4go.Flags.CommonMark);
  });

  it('RendererFlags bitmask', () => {
    const R = md4go.RendererFlags;
    assert.equal(R.FlagDebug, 0x0001);
    assert.equal(R.FlagVerbatimEntities, 0x0002);
    assert.equal(R.FlagSkipUTF8BOM, 0x0004);
    assert.equal(R.FlagXHTML, 0x0008);
    assert.equal(R.FlagNoXHTMLEscaping, 0x0010);
  });
});

// ═══════════════════════════════════════════════════════════════════════
// Edge Cases
// ═══════════════════════════════════════════════════════════════════════

describe('Edge Cases', () => {
  it('HTML passthrough', () => {
    // Markdown passes raw HTML through by default; verify it's preserved.
    const got = md4go.parseToHTML('<span class="x">hi</span>');
    assert.ok(got.includes('<span'), `should preserve HTML: ${got}`);
  });

  it('unicode emoji', () => {
    const got = md4go.parseToHTML('🚀 rocket');
    assert.ok(got.includes('🚀'), got);
  });

  it('hard break (trailing two spaces)', () => {
    // Two trailing spaces + newline → hard break <br />.
    const got = md4go.parseToHTML('line1  \nline2');
    assert.ok(got.includes('<br'), `hard break expected: ${got}`);
  });

  it('paragraph break (blank line)', () => {
    const got = md4go.parseToHTML('p1\n\np2');
    // Two paragraphs separated by blank line.
    const count = (got.match(/<p>/g) || []).length;
    assert.equal(count, 2, `expected 2 paragraphs in: ${got}`);
  });

  it('nested formatting', () => {
    const got = md4go.parseToHTML('***bold italic***');
    assert.ok(got.includes('<strong>') || got.includes('<em>'), got);
  });

  it('blockquote', () => {
    const got = md4go.parseToHTML('> quoted');
    assert.ok(got.includes('<blockquote>'), got);
  });

  it('horizontal rule', () => {
    const got = md4go.parseToHTML('---');
    assert.ok(got.includes('<hr'), got);
  });
});
