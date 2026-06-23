package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"md4go/html"
	"md4go/parser"
	"md4go/text"
)

func main() {
	useStream := flag.Bool("stream", false, "incremental input mode (io.Reader, low memory)")
	outputHTML := flag.Bool("html", false, "output HTML instead of plain text")
	compat := flag.String("compat", "none", "compatibility mode: none, goldmark")
	flag.Parse()

	// Default to GitHub dialect — mirrors md4c's MD_DIALECT_GITHUB default.
	textFlags := parser.DialectGitHub
	htmlFlags := parser.DialectGitHub

	switch *compat {
	case "goldmark":
		textFlags |= parser.GoldmarkCompat
		htmlFlags |= parser.GoldmarkCompat
	case "none", "":
		// default: GFM standard behavior
	default:
		fmt.Fprintf(os.Stderr, "unknown compat mode: %s (use none or goldmark)\n", *compat)
		os.Exit(1)
	}

	textOpts := []text.Option{text.WithFlags(textFlags)}
	htmlOpts := []html.Option{html.WithFlags(htmlFlags)}

	if *outputHTML {
		runHTML(htmlOpts, *useStream)
	} else {
		runPlainText(textOpts, *useStream)
	}
}

func runPlainText(opts []text.Option, useStream bool) {
	if useStream {
		if err := text.ConvertStream(os.Stdin, os.Stdout, opts...); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := text.Convert(b, os.Stdout, opts...); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
}

func runHTML(opts []html.Option, _ bool) {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := html.Convert(b, os.Stdout, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
