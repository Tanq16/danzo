package utils

// GlobalDebugFlag is set by the cobra root command when --debug is passed.
var GlobalDebugFlag bool

// GlobalForAIFlag is set by the cobra root command when --for-ai is passed.
// When true, output uses plain text prefixes and input reads from stdin.
var GlobalForAIFlag bool
