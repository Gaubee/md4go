// Package extension provides runtime extensions for the md4go parser.
//
// Each extension implements parser.Extender and registers its syntax handlers
// (block triggers, mark characters, flags) with the parser at construction time.
// This runtime injection mechanism enables third-party extensions without
// modifying the parser core.
//
// Usage:
//
//	md := md4go.New(md4go.WithExtensions(extension.GFM...))
//
// Or individually:
//
//	md := md4go.New(md4go.WithExtensions(
//	    &extension.Strikethrough{},
//	    &extension.Table{},
//	))
package extension
