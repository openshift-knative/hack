package log

import "time"

// TimedLogger will add current time to all messages.
type TimedLogger struct {
	Time   func() time.Time
	Format *string
	Logger
}

func (l TimedLogger) Println(v ...interface{}) {
	vars := prependText(l.formattedTime(), v)
	l.Logger.Println(vars...)
}

func (l TimedLogger) Printf(format string, v ...interface{}) {
	format = l.formattedTime() + " " + format
	l.Logger.Printf(format, v...)
}

func (l TimedLogger) formattedTime() string {
	ts := time.Now()
	if l.Time != nil {
		ts = l.Time()
	}
	format := time.StampMilli
	if l.Format != nil {
		format = *l.Format
	}
	return ts.Format(format)
}
