// Package proc starts child processes the same way on every OS: no
// console window on Windows (the packaged app is a GUI program, so each
// git or node call would otherwise flash one), and a group that holds the
// child and everything it starts so they can be stopped together.
package proc
