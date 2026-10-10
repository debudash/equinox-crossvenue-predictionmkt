// Package cache holds the in-memory feed snapshot built from SQLite for the
// latest completed cycle and swaps it atomically. It never calls a venue and
// never writes.
package cache
