//go:build linux

// Linux-specific tests for ExtractPeerCredentials are covered by peercred_test.go,
// which compiles on all platforms and exercises the happy path and the
// non-Unix connection error path. This file satisfies the goframe requirement
// that every .go source file has a corresponding _test.go file.
package ipc
