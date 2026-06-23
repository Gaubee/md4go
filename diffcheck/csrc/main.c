/*
 * md4c-plain: Use md4c library to parse Markdown and output plain text.
 *
 * Isolated tool — does not modify any existing source in md4c/ or the Go codebase.
 * Compile: gcc -O2 -I../../md4c/src -o md4c-plain main.c ../../md4c/src/md4c.c
 * Usage:   ./md4c-plain [--gfm|--commonmark] [file]
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "md4c.h"

typedef struct {
    FILE* out;
    int in_code_block;
    int in_html_block;
    int table_col;
    int in_table_row;  /* inside TR */
    int in_td;         /* inside TH or TD */
    int in_li;
    int list_is_tight;
    int need_nl;    /* need newline before next text */
    int need_blank; /* need blank line before next block text */
} PlainCtx;

static void flush_pending(PlainCtx* ctx)
{
    if (ctx->need_blank) {
        fputc('\n', ctx->out);
        ctx->need_blank = 0;
        ctx->need_nl = 0;
    } else if (ctx->need_nl) {
        fputc('\n', ctx->out);
        ctx->need_nl = 0;
    }
}

static int enter_block(MD_BLOCKTYPE type, void* detail, void* userdata)
{
    PlainCtx* ctx = (PlainCtx*)userdata;

    switch (type) {
    case MD_BLOCK_QUOTE:
        ctx->need_blank = 1;
        break;
    case MD_BLOCK_UL: {
        MD_BLOCK_UL_DETAIL* d = (MD_BLOCK_UL_DETAIL*)detail;
        ctx->list_is_tight = d->is_tight;
        break;
    }
    case MD_BLOCK_OL: {
        MD_BLOCK_OL_DETAIL* d = (MD_BLOCK_OL_DETAIL*)detail;
        ctx->list_is_tight = d->is_tight;
        break;
    }
    case MD_BLOCK_LI:
        ctx->in_li = 1;
        break;
    case MD_BLOCK_H:
        ctx->need_blank = 1;
        break;
    case MD_BLOCK_CODE:
        ctx->in_code_block = 1;
        ctx->need_blank = 1;
        break;
    case MD_BLOCK_HTML:
        ctx->in_html_block = 1;
        ctx->need_blank = 1;
        break;
    case MD_BLOCK_P:
        if (ctx->in_td) {
            /* paragraph inside table cell: no separator needed */
        } else if (ctx->in_li && ctx->list_is_tight) {
            if (ctx->need_nl) {
                fputc('\n', ctx->out);
                ctx->need_nl = 0;
            }
        } else {
            ctx->need_blank = 1;
        }
        break;
    case MD_BLOCK_HR:
        ctx->need_blank = 1;
        break;
    case MD_BLOCK_TR:
        ctx->in_table_row = 1;
        ctx->table_col = 0;
        flush_pending(ctx);
        break;
    case MD_BLOCK_TH:
    case MD_BLOCK_TD:
        ctx->in_td = 1;
        if (ctx->table_col > 0)
            fputc('\t', ctx->out);
        ctx->table_col++;
        break;
    case MD_BLOCK_FOOTNOTE_DEF_SECTION:
    case MD_BLOCK_FOOTNOTE_DEF:
    case MD_BLOCK_ADMONITION:
        ctx->need_blank = 1;
        break;
    default:
        break;
    }
    return 0;
}

static int leave_block(MD_BLOCKTYPE type, void* detail, void* userdata)
{
    PlainCtx* ctx = (PlainCtx*)userdata;
    (void)detail;

    switch (type) {
    case MD_BLOCK_QUOTE:
    case MD_BLOCK_H:
    case MD_BLOCK_CODE:
    case MD_BLOCK_HTML:
    case MD_BLOCK_HR:
    case MD_BLOCK_TABLE:
    case MD_BLOCK_FOOTNOTE_DEF:
        ctx->need_nl = 1;
        break;
    case MD_BLOCK_P:
        /* suppress newline inside table cells */
        if (!ctx->in_td)
            ctx->need_nl = 1;
        break;
    case MD_BLOCK_TR:
        ctx->in_table_row = 0;
        ctx->need_nl = 1;
        break;
    case MD_BLOCK_TH:
    case MD_BLOCK_TD:
        ctx->in_td = 0;
        break;
    case MD_BLOCK_UL:
    case MD_BLOCK_OL:
        ctx->list_is_tight = 0;
        ctx->need_blank = 1;
        break;
    case MD_BLOCK_LI:
        ctx->in_li = 0;
        ctx->need_nl = 1;
        break;
    default:
        ctx->need_nl = 1;
        break;
    }
    return 0;
}

static int enter_span(MD_SPANTYPE type, void* detail, void* userdata)
{
    (void)type; (void)detail; (void)userdata;
    return 0;
}

static int leave_span(MD_SPANTYPE type, void* detail, void* userdata)
{
    (void)type; (void)detail; (void)userdata;
    return 0;
}

static int text_cb(MD_TEXTTYPE type, const MD_CHAR* text, MD_SIZE size, void* userdata)
{
    PlainCtx* ctx = (PlainCtx*)userdata;

    switch (type) {
    case MD_TEXT_NULLCHAR:
        flush_pending(ctx);
        /* U+FFFD in UTF-8 */
        fputs("\xEF\xBF\xBD", ctx->out);
        break;
    case MD_TEXT_BR:
    case MD_TEXT_SOFTBR:
        fputc('\n', ctx->out);
        ctx->need_nl = 0;
        break;
    case MD_TEXT_NORMAL:
    case MD_TEXT_CODE:
    case MD_TEXT_HTML:
    case MD_TEXT_LATEXMATH:
    case MD_TEXT_ENTITY:
    default:
        flush_pending(ctx);
        fwrite(text, 1, size, ctx->out);
        break;
    }
    return 0;
}

static void debug_log(const char* msg, void* userdata)
{
    (void)userdata;
    fprintf(stderr, "MD4C debug: %s\n", msg);
}

static char* read_file(FILE* f, size_t* out_size)
{
    size_t cap = 64 * 1024, len = 0;
    char* buf = malloc(cap);
    if (!buf) return NULL;
    for (;;) {
        if (len >= cap) {
            cap += cap / 2;
            buf = realloc(buf, cap);
            if (!buf) return NULL;
        }
        size_t n = fread(buf + len, 1, cap - len, f);
        if (n == 0) break;
        len += n;
    }
    *out_size = len;
    return buf;
}

int main(int argc, char** argv)
{
    FILE* in = stdin;
    const char* in_path = NULL;
    unsigned parser_flags = MD_DIALECT_GITHUB;

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--commonmark") == 0)
            parser_flags = MD_DIALECT_COMMONMARK;
        else if (strcmp(argv[i], "--github") == 0 || strcmp(argv[i], "--gfm") == 0)
            parser_flags = MD_DIALECT_GITHUB;
        else
            in_path = argv[i];
    }

    if (in_path && strcmp(in_path, "-") != 0) {
        in = fopen(in_path, "rb");
        if (!in) {
            fprintf(stderr, "Cannot open %s\n", in_path);
            return 1;
        }
    }

    size_t input_size = 0;
    char* input = read_file(in, &input_size);
    if (!input) {
        fprintf(stderr, "Failed to read input\n");
        return 1;
    }
    if (in != stdin) fclose(in);

    PlainCtx ctx = {0};
    ctx.out = stdout;

    MD_PARSER parser = {
        .abi_version = 0,
        .flags       = parser_flags,
        .enter_block = enter_block,
        .leave_block = leave_block,
        .enter_span  = enter_span,
        .leave_span  = leave_span,
        .text        = text_cb,
        .debug_log   = debug_log,
        .syntax      = NULL,
    };

    int ret = md_parse(input, (MD_SIZE)input_size, &parser, &ctx);
    if (ret != 0)
        fprintf(stderr, "md_parse failed with code %d\n", ret);

    if (ctx.need_nl)
        fputc('\n', stdout);

    free(input);
    return ret;
}
