/**
 * md4go.js — WebAssembly loader for md4go.
 *
 * Quick start:
 *
 *   import { initMd4go } from './wasm/md4go.js';
 *   const md4go = await initMd4go();
 *
 *   // One-shot conversion
 *   md4go.parseToHTML("# Hello");           // → <h1>Hello</h1>
 *   md4go.parseToText("- item");            // → item
 *
 *   // Parser reuse — recommended for repeated parsing (e.g. live preview)
 *   const p = md4go.createParser(md4go.Flags.GitHub);
 *   p.parseToHTML("# first");               // reuses parser instance
 *   p.parseToHTML("## second");
 *   p.dispose();
 *
 *   // Stream accumulator — feed chunks from fetch/FileReader
 *   const s = md4go.createStreamParser(md4go.Flags.GitHub);
 *   s.write("# big doc start\n\n");
 *   s.write("rest of content...\n");
 *   const html = s.finishHTML();
 *   s.dispose();
 *
 *   // Custom renderer — extract structured data (场景5)
 *   const links = [];
 *   md4go.parseWithRenderer("see [link](url)", {
 *     enterSpan(type, detail) {
 *       if (type === md4go.SpanType.SpanLink) links.push(detail.href);
 *     },
 *   });
 *
 * Build:
 *   GOOS=js GOARCH=wasm go build -o md4go.wasm ./wasm
 *   cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
 */

const DEFAULT_WASM_PATH = './md4go.wasm';

// ─── Type constants (mirrors Go ast package) ─────────────────────────

/** BlockType identifies block-level document elements. */
const BlockType = Object.freeze({
  BlockDoc:               0,
  BlockQuote:             1,
  BlockUL:                2,
  BlockOL:                3,
  BlockLI:                4,
  BlockHR:                5,
  BlockH:                 6,
  BlockCode:              7,
  BlockHTML:              8,
  BlockP:                 9,
  BlockTable:            10,
  BlockTHead:            11,
  BlockTBody:            12,
  BlockTR:               13,
  BlockTH:               14,
  BlockTD:               15,
  BlockFootnoteDefSection: 16,
  BlockFootnoteDef:       17,
  BlockAdmonition:        18,
});

/** SpanType identifies inline span elements. */
const SpanType = Object.freeze({
  SpanEm:               0,
  SpanStrong:           1,
  SpanLink:             2,
  SpanImg:              3,
  SpanCode:             4,
  SpanDel:              5,
  SpanLatexMath:        6,
  SpanLatexMathDisplay: 7,
  SpanWikilink:         8,
  SpanU:                9,
  SpanSpoiler:         10,
  SpanSuperscript:     11,
  SpanSubscript:       12,
  SpanFootnoteRef:     13,
  SpanMark:            14,
});

/** TextType classifies text content within a span. */
const TextType = Object.freeze({
  TextNormal:    0,
  TextNullChar:  1,
  TextBR:        2,
  TextSoftBR:    3,
  TextEntity:    4,
  TextCode:      5,
  TextHTML:      6,
  TextLatexMath: 7,
});

// ─── Flags constants ─────────────────────────────────────────────────

/** Parser flag presets. */
const Flags = Object.freeze({
  CommonMark: 0,
  GitHub: 1,
});

/** HTML renderer flags bitmask. */
const RendererFlags = Object.freeze({
  FlagDebug:            0x0001,
  FlagVerbatimEntities: 0x0002,
  FlagSkipUTF8BOM:      0x0004,
  FlagXHTML:            0x0008,
  FlagNoXHTMLEscaping:  0x0010,
});

// ─── Overload helpers ────────────────────────────────────────────────

const CALLBACK_KEYS = ['enterBlock', 'leaveBlock', 'enterSpan', 'leaveSpan', 'text'];

function isCallbacksObject(obj) {
  if (typeof obj !== 'object' || obj === null) return false;
  return CALLBACK_KEYS.some(k => typeof obj[k] === 'function');
}

// ─── WASM loader ─────────────────────────────────────────────────────

/**
 * Initialize md4go WebAssembly and return the API.
 *
 * @param {string} [wasmPath='./md4go.wasm']
 * @returns {Promise<Md4goAPI>}
 */
export async function initMd4go(wasmPath = DEFAULT_WASM_PATH) {
  const go = new Go();

  let wasmInst;
  if (typeof WebAssembly.instantiateStreaming === 'function' &&
      wasmPath.startsWith('http')) {
    wasmInst = await WebAssembly.instantiateStreaming(fetch(wasmPath), go.importObject);
  } else {
    const resp = await fetch(wasmPath);
    const bytes = await resp.arrayBuffer();
    wasmInst = await WebAssembly.instantiate(bytes, go.importObject);
  }

  go.run(wasmInst.instance);

  await new Promise(resolve => {
    const check = () => {
      if (globalThis.md4goParseToHTML && globalThis.md4goParseToText) {
        resolve();
      } else {
        setTimeout(check, 10);
      }
    };
    check();
  });

  // ── One-shot APIs ──────────────────────────────────────────────

  function parseToHTML(md, flagsOrOptions = 1) {
    if (typeof flagsOrOptions === 'object' && flagsOrOptions !== null) {
      return parseToHTMLWithOptions(md, flagsOrOptions);
    }
    return globalThis.md4goParseToHTML(md, flagsOrOptions);
  }

  function parseToText(md, flags = 1) {
    return globalThis.md4goParseToText(md, flags);
  }

  function parseToHTMLWithOptions(md, options = {}) {
    if (globalThis.md4goParseToHTMLWithOptions) {
      return globalThis.md4goParseToHTMLWithOptions(md, options);
    }
    const flags = options.flags ?? 1;
    return globalThis.md4goParseToHTML(md, flags);
  }

  function parseWithRenderer(md, flagsOrCallbacks = 1, callbacks) {
    let flags = 1;
    let cbs = null;

    if (isCallbacksObject(flagsOrCallbacks)) {
      cbs = flagsOrCallbacks;
    } else {
      flags = flagsOrCallbacks;
      cbs = callbacks || null;
    }

    if (!globalThis.md4goParseWithRenderer) {
      return { error: 'md4goParseWithRenderer not available (rebuild WASM required)' };
    }
    return globalThis.md4goParseWithRenderer(md, flags, cbs);
  }

  // ── Parser reuse ───────────────────────────────────────────────

  /**
   * Create a reusable parser. Reuses the same parser instance across
   * parse calls, avoiding repeated extension registration overhead.
   * Recommended for live preview or batch processing.
   *
   * @param {number} [flags=1] - Parser flags (0=CommonMark, 1=GFM)
   * @returns {{parseToHTML, parseToText, parseWithRenderer, dispose}}
   */
  function createParser(flags = 1) {
    if (!globalThis.md4goCreateParser) {
      return {
        parseToHTML: () => '',
        parseToText: () => '',
        parseWithRenderer: () => ({ error: 'rebuild WASM required' }),
        dispose: () => {},
      };
    }
    return globalThis.md4goCreateParser(flags);
  }

  // ── Stream accumulator ─────────────────────────────────────────

  /**
   * Create a chunk-accumulating stream parser for large documents.
   * Feed chunks via write(), then call finishHTML() or finishText()
   * to parse the complete document.
   *
   * @param {number} [flags=1] - Parser flags (0=CommonMark, 1=GFM)
   * @returns {{write, finishHTML, finishText, dispose}}
   */
  function createStreamParser(flags = 1) {
    if (!globalThis.md4goCreateStreamParser) {
      return {
        write: () => {},
        finishHTML: () => '',
        finishText: () => '',
        dispose: () => {},
      };
    }
    return globalThis.md4goCreateStreamParser(flags);
  }

  return {
    // One-shot
    parseToHTML,
    parseToText,
    parseToHTMLWithOptions,
    parseWithRenderer,

    // Lifecycle
    createParser,
    createStreamParser,

    // Type constants
    BlockType,
    SpanType,
    TextType,

    // Flag constants
    Flags,
    RendererFlags,
  };
}
