package log

// Logger is used to log messages to output.
type Logger interface {
	Println(v ...interface{})
	Printf(format string, v ...interface{})
}
