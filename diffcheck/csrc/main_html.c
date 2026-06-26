/*
 * md4c-html: Use md4c's built-in HTML renderer to parse Markdown and
 * output XHTML (for alignment with md4go-html and goldmark's XHTML mode).
 *
 * Compile: gcc -O2 -I../../md4c/src -o md4c-html main_html.c ../../md4c/src/md4c.c ../../md4c/src/md4c-html.c ../../md4c/src/entity.c
 * Usage:   ./md4c-html [--gfm|--commonmark] [file]
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "md4c-html.h"

static void process_output(const MD_CHAR* text, MD_SIZE size, void* userdata)
{
    (void)userdata;
    fwrite(text, 1, size, stdout);
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
    unsigned renderer_flags = MD_HTML_FLAG_XHTML;

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

    int ret = md_html(input, (MD_SIZE)input_size, process_output, stdout,
                       parser_flags, renderer_flags);
    if (ret != 0)
        fprintf(stderr, "md_html failed with code %d\n", ret);

    /* add trailing newline to align with md4go-html output */
    fputc('\n', stdout);

    free(input);
    return ret;
}
