// This is a logger implementation that supports multiple log levels,
// multiple output destinations with configurable formats and levels
// for each.  It also supports granular output configuration to get
// more detailed logging for specific files/packages. Timber includes
// support for standard XML or JSON config files to get you started
// quickly.  It's also easy to configure in code if you want to DIY.
//
// Basic use:
//
//	import "timber"
//	timber.LoadConfiguration("timber.xml")
//	timber.Debug("Debug message!")
//
// IMPORTANT: timber has not default destination configured so log messages
// will be dropped until a destination is configured
//
// It can be used as a drop-in replacement for the standard logger
// by changing the log import statement from:
//
//	import "log"
//
// to
//
//	import log "timber"
//
// It can also be used as the output of the standard logger with
//
//	log.SetFlags(0)
//	log.SetOutput(timber.Global)
//
// Configuration in code is also simple:
//
//	timber.AddLogger(timber.ConfigLogger{
//		LogWriter: new(timber.ConsoleWriter),
//		Level:     timber.DEBUG,
//		Formatter: timber.NewPatFormatter("[%D %T] [%L] %S %M"),
//	})
//
// XML Config file:
//
//	<logging>
//	  <filter enabled="true">
//		<tag>stdout</tag>
//		<type>console</type>
//		<!-- level is (:?FINEST|FINE|DEBUG|TRACE|INFO|WARNING|ERROR) -->
//		<level>DEBUG</level>
//	  </filter>
//	  <filter enabled="true">
//		<tag>file</tag>
//		<type>file</type>
//		<level>FINEST</level>
//		<granular>
//		  <level>INFO</level>
//		  <path>path/to/package.FunctionName</path>
//		</granular>
//		<granular>
//		  <level>WARNING</level>
//		  <path>path/to/package</path>
//		</granular>
//		<property name="filename">log/server.log</property>
//		<property name="format">server [%D %T] [%L] %M</property>
//	  </filter>
//	  <filter enabled="false">
//		<tag>syslog</tag>
//		<type>socket</type>
//		<level>FINEST</level>
//		<property name="protocol">unixgram</property>
//		<property name="endpoint">/dev/log</property>
//	    <format name="pattern">%L %M</property>
//	  </filter>
//	</logging>
//
// The <tag> is ignored.
//
// To configure the pattern formatter all filters accept:
//
//	<format name="pattern">[%D %T] %L %M</format>
//
// Pattern format specifiers (not the same as log4go!):
//
//	%T - Time: 17:24:05.333 HH:MM:SS.ms
//	%t - Time: 17:24:05 HH:MM:SS
//	%D - Date: 2011-12-25 yyyy-mm-dd
//	%d - Date: 2011/12/25 yyyy/mm/dd
//	%L - Level (FNST, FINE, DEBG, TRAC, WARN, EROR, CRIT)
//	%S - Source: full runtime.Caller line and line number
//	%s - Short Source: just file and line number
//	%x - Extra Short Source: just file without .go suffix
//	%M - Message
//	%% - Percent sign
//	%P - Caller Path: packagePath.CallingFunctionName
//	%p - Caller Path: packagePath
//
// the string number prefixes are allowed e.g.: %10s will pad the source field to 10 spaces
// pattern defaults to %M
// Both log4go synatax of <property name="format"> and new <format name=type> are supported
// the property syntax will only ever support the pattern formatter
// To configure granulars:
//   - Create one or many <granular> within a filter
//   - Define a <level> and <path> within, where path can be path to package or path to
//     package.FunctionName. Function name definitions override package paths.
//
// Code Architecture:
// A MultiLogger <logging> which consists of many ConfigLoggers <filter>. ConfigLoggers have three properties:
// LogWriter <type>, Level (as a threshold) <level> and LogFormatter <format>.
//
// In practice, this means that you define ConfigLoggers with a LogWriter (where the log prints to
// eg. socket, file, stdio etc), the Level threshold, and a LogFormatter which formats the message
// before writing.  Because the LogFormatters and LogWriters are simple interfaces, it is easy to
// write your own custom implementations.
//
// Once configured, you only deal with the "Logger" interface and use the log methods in your code
//
// The motivation for this package grew from a need to make some changes to the functionality of
// log4go (which had already been integrated into a larger project).  I tried to maintain compatiblity
// with log4go for the interface and configuration.  The main issue I had with log4go was that each of
// logger types had incisistent and incompatible configuration.  I looked at contributing changes to
// log4go, but I would have needed to break existing use cases so I decided to do a rewrite from scratch.
package timber

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Level int

// Log levels
const (
	NONE Level = iota // NONE to be used for standard go log impl's
	FINEST
	FINE
	DEBUG
	TRACE
	INFO
	WARNING
	ERROR
	CRITICAL
)

// Default level passed to runtime.Caller by Timber, add to this if you wrap Timber in your own logging code
const DefaultFileDepth int = 3

// What gets printed for each Log level
var LevelStrings = [...]string{"", "FNST", "FINE", "DEBG", "TRAC", "INFO", "WARN", "EROR", "CRIT"}

// Full level names
var LongLevelStrings = []string{
	"NONE",
	"FINEST",
	"FINE",
	"DEBUG",
	"TRACE",
	"INFO",
	"WARNING",
	"ERROR",
	"CRITICAL",
}

// GetLevel returns a given level string as the actual Level value
func GetLevel(lvlString string) Level {
	for idx, str := range LongLevelStrings {
		if str == lvlString {
			return Level(idx)
		}
	}
	return Level(0)
}

// This explicitly defines the contract for a logger
// Not really useful except for documentation for
// writing an separate implementation
type Logger interface {
	// match log4go interface to drop-in replace
	Finest(arg0 interface{}, args ...interface{})
	Fine(arg0 interface{}, args ...interface{})
	Debug(arg0 interface{}, args ...interface{})
	Trace(arg0 interface{}, args ...interface{})
	Info(arg0 interface{}, args ...interface{})
	Warn(arg0 interface{}, args ...interface{}) error
	Error(arg0 interface{}, args ...interface{}) error
	Critical(arg0 interface{}, args ...interface{}) error
	Log(lvl Level, arg0 interface{}, args ...interface{})

	// support standard log too
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
	Panic(v ...interface{})
	Panicf(format string, v ...interface{})
	Panicln(v ...interface{})
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
	Fatalln(v ...interface{})

	// and govet-friendly versions of log4go
	Finestf(arg0 interface{}, args ...interface{})
	Finef(arg0 interface{}, args ...interface{})
	Debugf(arg0 interface{}, args ...interface{})
	Tracef(arg0 interface{}, args ...interface{})
	Infof(arg0 interface{}, args ...interface{})
	Warnf(arg0 interface{}, args ...interface{}) error
	Errorf(arg0 interface{}, args ...interface{}) error
	Criticalf(arg0 interface{}, args ...interface{}) error
	Logf(lvl Level, arg0 interface{}, args ...interface{})

	// allow passing of extra fields on the fly
	FinestEx(extra map[string]interface{}, arg0 interface{}, args ...interface{})
	FineEx(extra map[string]interface{}, arg0 interface{}, args ...interface{})
	DebugEx(extra map[string]interface{}, arg0 interface{}, args ...interface{})
	TraceEx(extra map[string]interface{}, arg0 interface{}, args ...interface{})
	InfoEx(extra map[string]interface{}, arg0 interface{}, args ...interface{})
	WarnEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error
	ErrorEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error
	CriticalEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error
	LogEx(extra map[string]interface{}, lvl Level, arg0 interface{}, args ...interface{})
}

// Not used
type LoggerConfig interface {
	// When set, messages with level < lvl will be ignored.  It's up to the implementor to keep the contract or not
	SetLevel(lvl Level)
	// Set the formatter for the log
	SetFormatter(formatter LogFormatter)
}

// Interface required for a log writer endpoint.  It's more or less a
// io.WriteCloser with no errors allowed to be returned and string
// instead of []byte.
//
// TODO: Maybe this should just be a standard io.WriteCloser?
type LogWriter interface {
	LogWrite(msg string)
	Close()
}

// This packs up all the message data and metadata. This structure
// will be passed to the LogFormatter
type LogRecord struct {
	Level       Level
	Timestamp   time.Time `json:"timestamp"`
	SourceFile  string
	SourceLine  int
	Message     string `json:"message"`
	FuncPath    string
	MethodPath  string
	PackagePath string
	HostName    string
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// Format a log message before writing
type LogFormatter interface {
	Format(rec *LogRecord) string
}

// Container a single log format/destination
type ConfigLogger struct {
	LogWriter LogWriter
	// Messages with level < Level will be ignored.  It's up to the implementor to keep the contract or not
	Level     Level
	Formatter LogFormatter
	Granulars map[string]Level
}

// Allow logging to multiple places
type MultiLogger interface {
	// returns an int that identifies the logger for future calls to SetLevel and SetFormatter
	AddLogger(logger ConfigLogger) int
	SetLogger(index int, logger ConfigLogger)
	// dynamically change level or format
	SetLevel(index int, lvl Level)
	SetFormatter(index int, formatter LogFormatter)
	Close()
}

//
//
//
// Implementation
//
//
//

// The Timber instance is the concrete implementation of the logger interfaces.
// New instances may be created, but usually you'll just want to use the default
// instance in Global
//
// NOTE: I don't supporting the log4go special handling of the first parameter based on type
// mainly cuz I don't think it's particularly useful (I kept passing a data string as the first
// param and expecting a Println-like output but that would always break expecting a format string)
// I also don't support the passing of the closure stuff
type Timber struct {
	writerConfigChan chan timberConfig
	recordChan       chan *LogRecord
	hasLogger        bool
	closeLatch       *sync.Once
	blackHole        chan int
	// This value is passed to runtime.Caller to get the file name/line and may require
	// tweaking if you want to wrap the logger
	FileDepth int
	Hostname  func() string
}

type timberAction int

const (
	actionAdd timberAction = iota
	actionSet
	actionModify
	actionQuit
)

type timberConfig struct {
	Action timberAction // type of config action
	Index  int          // only for modify
	Cfg    ConfigLogger // used for modify or add
	Ret    chan int     // only used for add
}

// Creates a new Timber logger that is ready to be configured
// With no subsequent configuration, nothing will be logged
func NewTimber() *Timber {
	t := new(Timber)
	t.writerConfigChan = make(chan timberConfig)
	t.recordChan = make(chan *LogRecord, 300)
	t.FileDepth = DefaultFileDepth
	t.closeLatch = &sync.Once{}
	t.blackHole = make(chan int)
	t.Hostname = func() string {
		h, _ := os.Hostname()
		return h
	}
	go t.asyncLumberJack()
	return t
}

func (t *Timber) asyncLumberJack() {
	var loggers []ConfigLogger = make([]ConfigLogger, 0, 2)
	loopIt := true
	for loopIt {
		select {
		case rec := <-t.recordChan:
			sendToLoggers(loggers, rec)
		case cfg := <-t.writerConfigChan:
			switch cfg.Action {
			case actionAdd:
				loggers = append(loggers, cfg.Cfg)
				cfg.Ret <- (len(loggers) - 1)
			case actionSet:
				// Old writer may want to flush, close handles etc.
				loggers[cfg.Index].LogWriter.Close()
				loggers[cfg.Index] = cfg.Cfg
			case actionModify:
			case actionQuit:
				close(t.blackHole)
				loopIt = false
				defer func() {
					cfg.Ret <- 0
				}()
			}
		} // select
	} // for
	// drain the log channel before closing (best effort)
	loopIt = true
	for loopIt {
		select {
		case rec := <-t.recordChan:
			sendToLoggers(loggers, rec)
		default:
			loopIt = false
		}
	}
	closeAllWriters(loggers)
}

func sendToLogger(rec *LogRecord, granLevel Level, formatted string, cLog ConfigLogger) bool {
	if rec.Level >= granLevel || granLevel == 0 {
		if formatted == "" {
			formatted = cLog.Formatter.Format(rec)
		}
		cLog.LogWriter.LogWrite(formatted)
		return true
	}
	return false
}

func sendToLoggers(loggers []ConfigLogger, rec *LogRecord) {
	formatted := ""
	for _, cLog := range loggers {
		// Find any function level definitions.
		gLevel, ok := cLog.Granulars[rec.FuncPath]
		if ok {
			sendToLogger(rec, gLevel, formatted, cLog)
			continue
		}
		// Find any package + method level definitions.
		gLevel, ok = cLog.Granulars[rec.MethodPath]
		if ok {
			sendToLogger(rec, gLevel, formatted, cLog)
			continue
		}
		// Find any package level definitions.
		gLevel, ok = cLog.Granulars[rec.PackagePath]
		if ok {
			sendToLogger(rec, gLevel, formatted, cLog)
			continue
		}
		// Use default definition
		sendToLogger(rec, cLog.Level, formatted, cLog)
	}
}

func closeAllWriters(cls []ConfigLogger) {
	for _, cLog := range cls {
		cLog.LogWriter.Close()
	}
}

// MultiLogger interface
func (t *Timber) AddLogger(logger ConfigLogger) int {
	tcChan := make(chan int, 1) // buffered
	tc := timberConfig{Action: actionAdd, Cfg: logger, Ret: tcChan}
	t.writerConfigChan <- tc
	return <-tcChan
}

func (t *Timber) SetLogger(index int, logger ConfigLogger) {
	tcChan := make(chan int, 1) // buffered
	tc := timberConfig{Action: actionSet, Cfg: logger, Ret: tcChan, Index: index}
	t.writerConfigChan <- tc
}

// MultiLogger interface
func (t *Timber) Close() {
	t.closeLatch.Do(func() {
		tcChan := make(chan int)
		tc := timberConfig{Action: actionQuit, Ret: tcChan}
		t.writerConfigChan <- tc
		<-tcChan // block for cloosing
	})
}

// Not yet implemented
func (t *Timber) SetLevel(index int, lvl Level) {
	// TODO
}

// Not yet implemented
func (t *Timber) SetFormatter(index int, formatter LogFormatter) {
	// TODO
}

// Logger interface
func (t *Timber) prepareAndSend(lvl Level, msg string, depth int) {
	var emptyExtra map[string]interface{}
	t.doPrepareAndSend(lvl, emptyExtra, msg, depth)
}

func (t *Timber) prepareAndSendEx(lvl Level, extra map[string]interface{}, msg string, depth int) {
	t.doPrepareAndSend(lvl, extra, msg, depth)
}

func (t *Timber) doPrepareAndSend(lvl Level, extra map[string]interface{}, msg string, depth int) {
	select {
	case <-t.blackHole:
		// the blackHole always blocks until we close
		// then it always succeeds so we avoid writing
		// to the closed channel
	default:
		t.recordChan <- t.prepare(lvl, extra, msg, depth+2) // +2 required to accommodate the prepareAndSend function(s) in the call stack
	}
}

// Return package.function into just the package component.
// Parse some.package/with/bits.Func or some.package/with/bits.(Type).Func
// and return the full pkg path and (if a method call) the method path too
func parseFuncName(funcName string) (string, string) {
	packagePath := ""
	methodPath := ""

	lastDot := strings.LastIndex(funcName, ".")
	packagePath = funcName[:lastDot]

	if packagePath[len(packagePath)-1] == ')' {
		methodPath = packagePath
		lastDot := strings.LastIndex(packagePath, ".")
		packagePath = packagePath[:lastDot]
	}
	return packagePath, methodPath
}

// makeTimeLogglyCompat takes a time converts it into a time parseable by loggly
// loggly can only parse up to 6 places of fractional seconds
func makeTimeLogglyCompat(t time.Time) time.Time {
	RFC3339Micro := "2006-01-02T15:04:05.999999Z07:00"
	tStr := t.UTC().Format(RFC3339Micro)
	tLoggly, _ := time.Parse(RFC3339Micro, tStr)
	return tLoggly
}

func (t *Timber) prepare(lvl Level, extra map[string]interface{}, msg string, depth int) *LogRecord {
	now := makeTimeLogglyCompat(time.Now())
	pc, file, line, _ := runtime.Caller(depth)
	funcPath := "_"
	packagePath := "_"
	methodPath := "_"
	me := runtime.FuncForPC(pc)
	if me != nil {
		funcPath = me.Name()
		packagePath, methodPath = parseFuncName(funcPath)
	}

	var hostname string
	if t.Hostname != nil {
		hostname = t.Hostname()
	}
	return &LogRecord{
		Level:       lvl,
		Timestamp:   now,
		SourceFile:  file,
		SourceLine:  line,
		Message:     msg,
		FuncPath:    funcPath,
		MethodPath:  methodPath,
		PackagePath: packagePath,
		HostName:    hostname,
		Extra:       extra,
	}
}

// This function allows a Timber instance to be used in the standard library
// log.SetOutput().  It is not a general Writer interface and assumes one
// message per call to Write. All messages are send at level INFO
func (t *Timber) Write(p []byte) (n int, err error) {
	t.prepareAndSend(INFO, string(bytes.TrimSpace(p)), 4)
	return len(p), nil
}

func (t *Timber) Finest(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(FINEST, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Fine(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(FINE, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Debug(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(DEBUG, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Trace(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(TRACE, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Info(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(INFO, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Warn(arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSend(WARNING, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) Error(arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSend(ERROR, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) Critical(arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) Log(lvl Level, arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(lvl, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}

// The govet printf family of warnings triggers on Erorr() and similar containing format strings
// Add more golike Foof() formatters. Other methods should be considered deprecated
func (t *Timber) Finestf(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(FINEST, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Finef(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(FINE, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Debugf(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(DEBUG, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Tracef(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(TRACE, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Infof(arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(INFO, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) Warnf(arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSend(WARNING, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) Errorf(arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSend(ERROR, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) Criticalf(arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) Logf(lvl Level, arg0 interface{}, args ...interface{}) {
	t.prepareAndSend(lvl, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}

// Print won't work well with a pattern_logger because it explicitly adds
// its own \n; so you'd have to write your own formatter to remove it
func (t *Timber) Print(v ...interface{}) {
	t.prepareAndSend(DEBUG, fmt.Sprint(v...), t.FileDepth)
}
func (t *Timber) Printf(format string, v ...interface{}) {
	t.prepareAndSend(DEBUG, fmt.Sprintf(format, v...), t.FileDepth)
}

// Println won't work well either with a pattern_logger because it explicitly adds
// its own \n; so you'd have to write your own formatter to not have 2 \n's
func (t *Timber) Println(v ...interface{}) {
	t.prepareAndSend(DEBUG, fmt.Sprintln(v...), t.FileDepth)
}
func (t *Timber) Panic(v ...interface{}) {
	msg := fmt.Sprint(v...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	panic(msg)
}
func (t *Timber) Panicf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	panic(msg)
}
func (t *Timber) Panicln(v ...interface{}) {
	msg := fmt.Sprintln(v...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	panic(msg)
}
func (t *Timber) Fatal(v ...interface{}) {
	msg := fmt.Sprint(v...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	t.Close()
	os.Exit(1)
}
func (t *Timber) Fatalf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	t.Close()
	os.Exit(1)
}
func (t *Timber) Fatalln(v ...interface{}) {
	msg := fmt.Sprintln(v...)
	t.prepareAndSend(CRITICAL, msg, t.FileDepth)
	t.Close()
	os.Exit(1)
}

func (t *Timber) FinestEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	t.prepareAndSendEx(FINEST, extra, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) FineEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	t.prepareAndSendEx(FINE, extra, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) DebugEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	t.prepareAndSendEx(DEBUG, extra, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) TraceEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	t.prepareAndSendEx(TRACE, extra, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) InfoEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	t.prepareAndSendEx(INFO, extra, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}
func (t *Timber) WarnEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSendEx(WARNING, extra, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) ErrorEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSendEx(ERROR, extra, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) CriticalEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error {
	msg := fmt.Sprintf(arg0.(string), args...)
	t.prepareAndSendEx(CRITICAL, extra, msg, t.FileDepth)
	return errors.New(msg)
}
func (t *Timber) LogEx(extra map[string]interface{}, lvl Level, arg0 interface{}, args ...interface{}) {
	t.prepareAndSendEx(lvl, extra, fmt.Sprintf(arg0.(string), args...), t.FileDepth)
}

//
//
// Default Instance
//
//

// Default Timber Instance (used for all the package level function calls)
var Global = NewTimber()

// Simple wrappers for Logger interface
func Finest(arg0 interface{}, args ...interface{})         { Global.Finest(arg0, args...) }
func Fine(arg0 interface{}, args ...interface{})           { Global.Fine(arg0, args...) }
func Debug(arg0 interface{}, args ...interface{})          { Global.Debug(arg0, args...) }
func Trace(arg0 interface{}, args ...interface{})          { Global.Trace(arg0, args...) }
func Info(arg0 interface{}, args ...interface{})           { Global.Info(arg0, args...) }
func Warn(arg0 interface{}, args ...interface{}) error     { return Global.Warn(arg0, args...) }
func Error(arg0 interface{}, args ...interface{}) error    { return Global.Error(arg0, args...) }
func Critical(arg0 interface{}, args ...interface{}) error { return Global.Critical(arg0, args...) }
func Log(lvl Level, arg0 interface{}, args ...interface{}) { Global.Log(lvl, arg0, args...) }

func Finestf(arg0 interface{}, args ...interface{})         { Global.Finestf(arg0, args...) }
func Finef(arg0 interface{}, args ...interface{})           { Global.Finef(arg0, args...) }
func Debugf(arg0 interface{}, args ...interface{})          { Global.Debugf(arg0, args...) }
func Tracef(arg0 interface{}, args ...interface{})          { Global.Tracef(arg0, args...) }
func Infof(arg0 interface{}, args ...interface{})           { Global.Infof(arg0, args...) }
func Warnf(arg0 interface{}, args ...interface{}) error     { return Global.Warnf(arg0, args...) }
func Errorf(arg0 interface{}, args ...interface{}) error    { return Global.Errorf(arg0, args...) }
func Criticalf(arg0 interface{}, args ...interface{}) error { return Global.Criticalf(arg0, args...) }
func Logf(lvl Level, arg0 interface{}, args ...interface{}) { Global.Logf(lvl, arg0, args...) }

func Print(v ...interface{})                 { Global.Print(v...) }
func Printf(format string, v ...interface{}) { Global.Printf(format, v...) }
func Println(v ...interface{})               { Global.Println(v...) }
func Panic(v ...interface{})                 { Global.Panic(v...) }
func Panicf(format string, v ...interface{}) { Global.Panicf(format, v...) }
func Panicln(v ...interface{})               { Global.Panicln(v...) }
func Fatal(v ...interface{})                 { Global.Fatal(v...) }
func Fatalf(format string, v ...interface{}) { Global.Fatalf(format, v...) }
func Fatalln(v ...interface{})               { Global.Fatalln(v...) }

func FinestEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	Global.FinestEx(extra, arg0, args...)
}
func FineEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	Global.FineEx(extra, arg0, args...)
}
func DebugEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	Global.DebugEx(extra, arg0, args...)
}
func TraceEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	Global.TraceEx(extra, arg0, args...)
}
func InfoEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) {
	Global.InfoEx(extra, arg0, args...)
}
func WarnEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error {
	return Global.WarnEx(extra, arg0, args...)
}
func ErrorEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error {
	return Global.ErrorEx(extra, arg0, args...)
}
func CriticalEx(extra map[string]interface{}, arg0 interface{}, args ...interface{}) error {
	return Global.CriticalEx(extra, arg0, args...)
}
func LogEx(extra map[string]interface{}, lvl Level, arg0 interface{}, args ...interface{}) {
	Global.LogEx(extra, lvl, arg0, args...)
}

func AddLogger(logger ConfigLogger) int { return Global.AddLogger(logger) }
func Close()                            { Global.Close() }

func LoadConfiguration(filename string)     { Global.LoadConfig(filename) }
func LoadXMLConfiguration(filename string)  { Global.LoadXMLConfig(filename) }
func LoadJSONConfiguration(filename string) { Global.LoadJSONConfig(filename) }
